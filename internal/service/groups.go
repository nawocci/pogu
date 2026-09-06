package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nawocci/pogu/internal/store"
)

var groupNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{0,62}[a-z0-9])?$`)

func (s *Service) validateGroupName(ctx context.Context, name string) (string, error) {
	name = strings.TrimSpace(name)
	if !groupNamePattern.MatchString(name) || strings.Contains(name, "/") {
		return "", fmt.Errorf("%w: group name must be 1-64 lowercase letters, digits, dots, underscores, or hyphens", ErrValidation)
	}
	if utf8.RuneCountInString(name) > 64 {
		return "", fmt.Errorf("%w: group name must be at most 64 characters", ErrValidation)
	}
	var exists int
	err := s.Store.DB.QueryRowContext(ctx, `SELECT 1 FROM providers WHERE prefix=? COLLATE NOCASE`, name).Scan(&exists)
	if err == nil {
		return "", fmt.Errorf("%w: group name conflicts with a provider prefix", ErrValidation)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	return name, nil
}

func validateGroupSelection(sel KeySelection) (KeySelection, error) {
	if sel == "" {
		return KeySelectionFirst, nil
	}
	if !sel.Valid() {
		return "", fmt.Errorf("%w: selection must be 'first' or 'round_robin'", ErrValidation)
	}
	return sel, nil
}

func scanGroup(row interface{ Scan(...any) error }) (Group, error) {
	var g Group
	var sel string
	var enabled, memberCount int
	var created, updated string
	err := row.Scan(&g.ID, &g.Name, &sel, &enabled, &memberCount, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Group{}, store.ErrNotFound
	}
	if err != nil {
		return Group{}, err
	}
	g.Selection = KeySelection(sel)
	if !g.Selection.Valid() {
		g.Selection = KeySelectionFirst
	}
	g.Enabled = enabled != 0
	g.MemberCount = memberCount
	g.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	g.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return g, nil
}

const groupSelect = `SELECT g.id, g.name, g.selection, g.enabled, (SELECT COUNT(1) FROM group_members gm WHERE gm.group_id=g.id), g.created_at, g.updated_at FROM groups g`

func (s *Service) CreateGroup(ctx context.Context, name string, enabled bool, selection KeySelection) (Group, error) {
	name, err := s.validateGroupName(ctx, name)
	if err != nil {
		return Group{}, err
	}
	sel, err := validateGroupSelection(selection)
	if err != nil {
		return Group{}, err
	}
	now := store.Now()
	result, err := s.Store.DB.ExecContext(ctx,
		`INSERT INTO groups(name,selection,enabled,created_at,updated_at) VALUES(?,?,?,?,?)`,
		name, sel, boolInt(enabled), now, now)
	if err != nil {
		if isUnique(err) {
			return Group{}, fmt.Errorf("%w: group name already exists", ErrAlreadyExists)
		}
		return Group{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Group{}, err
	}
	return s.GetGroup(ctx, id)
}

func (s *Service) GetGroup(ctx context.Context, id int64) (Group, error) {
	return scanGroup(s.Store.DB.QueryRowContext(ctx, groupSelect+` WHERE g.id=?`, id))
}

func (s *Service) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.Store.DB.QueryContext(ctx, groupSelect+` ORDER BY g.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Group, 0)
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Service) UpdateGroup(ctx context.Context, id int64, name string, enabled bool, selection KeySelection) (Group, error) {
	if _, err := s.GetGroup(ctx, id); err != nil {
		return Group{}, err
	}
	name, err := s.validateGroupName(ctx, name)
	if err != nil {
		return Group{}, err
	}
	sel, err := validateGroupSelection(selection)
	if err != nil {
		return Group{}, err
	}
	result, err := s.Store.DB.ExecContext(ctx,
		`UPDATE groups SET name=?,selection=?,enabled=?,updated_at=? WHERE id=?`,
		name, sel, boolInt(enabled), store.Now(), id)
	if err != nil {
		if isUnique(err) {
			return Group{}, fmt.Errorf("%w: group name already exists", ErrAlreadyExists)
		}
		return Group{}, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return Group{}, store.ErrNotFound
	}
	return s.GetGroup(ctx, id)
}

func (s *Service) SetGroupSelection(ctx context.Context, id int64, selection KeySelection) (Group, error) {
	sel, err := validateGroupSelection(selection)
	if err != nil {
		return Group{}, err
	}
	result, err := s.Store.DB.ExecContext(ctx,
		`UPDATE groups SET selection=?,updated_at=? WHERE id=?`, sel, store.Now(), id)
	if err != nil {
		return Group{}, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return Group{}, store.ErrNotFound
	}
	return s.GetGroup(ctx, id)
}

func (s *Service) DeleteGroup(ctx context.Context, id int64) error {
	result, err := s.Store.DB.ExecContext(ctx, `DELETE FROM groups WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

const groupMemberSelect = `SELECT gm.id, gm.group_id, gm.model_id, gm.position, gm.enabled, gm.created_at, gm.updated_at, p.name, p.prefix, m.name FROM group_members gm JOIN models m ON m.id=gm.model_id JOIN providers p ON p.id=m.provider_id`

func scanGroupMember(row interface{ Scan(...any) error }) (GroupMember, error) {
	var gm GroupMember
	var enabled int
	var created, updated string
	err := row.Scan(&gm.ID, &gm.GroupID, &gm.ModelID, &gm.Position, &enabled, &created, &updated, &gm.Provider, &gm.Prefix, &gm.ModelName)
	if errors.Is(err, sql.ErrNoRows) {
		return GroupMember{}, store.ErrNotFound
	}
	if err != nil {
		return GroupMember{}, err
	}
	gm.Enabled = enabled != 0
	gm.PublicID = gm.Prefix + "/" + gm.ModelName
	gm.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	gm.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return gm, nil
}

func (s *Service) ListGroupMembers(ctx context.Context, groupID int64) ([]GroupMember, error) {
	if _, err := s.GetGroup(ctx, groupID); err != nil {
		return nil, err
	}
	rows, err := s.Store.DB.QueryContext(ctx, groupMemberSelect+` WHERE gm.group_id=? ORDER BY gm.position, gm.id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]GroupMember, 0)
	for rows.Next() {
		gm, err := scanGroupMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, gm)
	}
	return out, rows.Err()
}

func (s *Service) AddGroupMember(ctx context.Context, groupID, modelID int64) (GroupMember, error) {
	if _, err := s.GetGroup(ctx, groupID); err != nil {
		return GroupMember{}, err
	}
	if _, err := s.GetModel(ctx, modelID); err != nil {
		return GroupMember{}, err
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return GroupMember{}, err
	}
	defer tx.Rollback()
	var maxPos int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) FROM group_members WHERE group_id=?`, groupID).Scan(&maxPos); err != nil {
		return GroupMember{}, err
	}
	now := store.Now()
	result, err := tx.ExecContext(ctx,
		`INSERT INTO group_members(group_id,model_id,position,enabled,created_at,updated_at) VALUES(?,?,?,1,?,?)`,
		groupID, modelID, maxPos+1, now, now)
	if err != nil {
		if isUnique(err) {
			return GroupMember{}, fmt.Errorf("%w: model is already a member of this group", ErrAlreadyExists)
		}
		return GroupMember{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return GroupMember{}, err
	}
	if err := tx.Commit(); err != nil {
		return GroupMember{}, err
	}
	return scanGroupMember(s.Store.DB.QueryRowContext(ctx, groupMemberSelect+` WHERE gm.id=?`, id))
}

func (s *Service) UpdateGroupMember(ctx context.Context, groupID, memberID int64, enabled bool) (GroupMember, error) {
	result, err := s.Store.DB.ExecContext(ctx,
		`UPDATE group_members SET enabled=?,updated_at=? WHERE id=? AND group_id=?`,
		boolInt(enabled), store.Now(), memberID, groupID)
	if err != nil {
		return GroupMember{}, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return GroupMember{}, store.ErrNotFound
	}
	return scanGroupMember(s.Store.DB.QueryRowContext(ctx, groupMemberSelect+` WHERE gm.id=?`, memberID))
}

func (s *Service) DeleteGroupMember(ctx context.Context, groupID, memberID int64) error {
	result, err := s.Store.DB.ExecContext(ctx, `DELETE FROM group_members WHERE id=? AND group_id=?`, memberID, groupID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Service) ReorderGroupMembers(ctx context.Context, groupID int64, memberIDs []int64) error {
	members, err := s.ListGroupMembers(ctx, groupID)
	if err != nil {
		return err
	}
	if len(memberIDs) != len(members) {
		return fmt.Errorf("%w: member order must include every member exactly once", ErrValidation)
	}
	current := make(map[int64]bool, len(members))
	for _, m := range members {
		current[m.ID] = true
	}
	for _, id := range memberIDs {
		if !current[id] {
			return fmt.Errorf("%w: member order must include every member exactly once", ErrValidation)
		}
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := store.Now()
	for position, id := range memberIDs {
		if _, err := tx.ExecContext(ctx, `UPDATE group_members SET position=?,updated_at=? WHERE id=? AND group_id=?`, position, now, id, groupID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type groupCandidate struct {
	Route Route
	Keys  []ProviderKey
}

func (s *Service) ResolveGroupTargets(ctx context.Context, name, protocol string) (Group, []groupCandidate, error) {
	group, err := scanGroup(s.Store.DB.QueryRowContext(ctx, groupSelect+` WHERE g.name=? COLLATE NOCASE`, name))
	if errors.Is(err, store.ErrNotFound) {
		return Group{}, nil, ErrUnknownRoute
	}
	if err != nil {
		return Group{}, nil, err
	}
	if !group.Enabled {
		return Group{}, nil, ErrUnknownRoute
	}
	rows, err := s.Store.DB.QueryContext(ctx, groupMemberSelect+` WHERE gm.group_id=? ORDER BY gm.position, gm.id`, group.ID)
	if err != nil {
		return Group{}, nil, err
	}
	defer rows.Close()
	var members []GroupMember
	for rows.Next() {
		gm, err := scanGroupMember(rows)
		if err != nil {
			return Group{}, nil, err
		}
		members = append(members, gm)
	}
	if err := rows.Err(); err != nil {
		return Group{}, nil, err
	}
	var out []groupCandidate
	for _, gm := range members {
		if !gm.Enabled {
			continue
		}
		route, err := s.ResolveRoute(ctx, gm.PublicID)
		if err != nil {
			continue
		}
		keys, err := s.EligibleKeys(ctx, route.Provider.ID)
		if err != nil || len(keys) == 0 {
			continue
		}
		out = append(out, groupCandidate{Route: route, Keys: keys})
	}
	if len(out) == 0 {
		return group, nil, ErrNoGroupTargets
	}
	if group.Selection == KeySelectionRoundRobin && len(out) > 1 {
		cursor := s.nextRoundRobinCursor(group.ID, len(out))
		rotated := make([]groupCandidate, len(out))
		for i := range out {
			rotated[i] = out[(cursor+i)%len(out)]
		}
		return group, rotated, nil
	}
	return group, out, nil
}
