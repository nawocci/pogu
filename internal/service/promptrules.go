package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nawocci/pogu/internal/store"
)

// Prompt rule targets may point at a logical model group or a concrete model.
const (
	PromptTargetGroup = "group"
	PromptTargetModel = "model"
)

const maxPromptRuleNameRunes = 64

// PromptRuleTarget associates a rule with a group or model.
type PromptRuleTarget struct {
	TargetType string `json:"target_type"`
	TargetID   int64  `json:"target_id"`
	Label      string `json:"label"`
}

// PromptRule is a named system prompt applied to the targets it claims.
type PromptRule struct {
	ID        int64              `json:"id"`
	Name      string             `json:"name"`
	Prompt    string             `json:"prompt"`
	Inherit   bool               `json:"inherit"`
	Enabled   bool               `json:"enabled"`
	Position  int                `json:"position"`
	Targets   []PromptRuleTarget `json:"targets"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

// PromptRuleInput is the write shape for create/update.
type PromptRuleInput struct {
	Name    string             `json:"name"`
	Prompt  string             `json:"prompt"`
	Inherit bool               `json:"inherit"`
	Enabled bool               `json:"enabled"`
	Targets []PromptRuleTarget `json:"targets"`
}

// PromptSegment is one resolved prompt contribution, in injection order.
type PromptSegment struct {
	RuleID   int64
	RuleName string
	Text     string
}

// promptRule is the cached, target-resolved form of a rule.
type promptRule struct {
	ID       int64
	Name     string
	Prompt   string
	Inherit  bool
	Enabled  bool
	Position int
	Groups   map[int64]bool
	Models   map[int64]bool
}

const promptRuleSelect = `SELECT r.id, r.name, r.prompt, r.inherit, r.enabled, r.position, r.created_at, r.updated_at FROM prompt_rules r`

func scanPromptRule(row interface{ Scan(...any) error }) (PromptRule, error) {
	var r PromptRule
	var inherit, enabled int
	var created, updated string
	err := row.Scan(&r.ID, &r.Name, &r.Prompt, &inherit, &enabled, &r.Position, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return PromptRule{}, store.ErrNotFound
	}
	if err != nil {
		return PromptRule{}, err
	}
	r.Inherit = inherit != 0
	r.Enabled = enabled != 0
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	r.Targets = []PromptRuleTarget{}
	return r, nil
}

const promptTargetSelect = `SELECT t.rule_id, t.target_type, t.target_id,
	CASE t.target_type WHEN 'group' THEN g.name ELSE COALESCE(p.prefix || '/' || m.name, '') END
	FROM prompt_rule_targets t
	LEFT JOIN groups g ON t.target_type = 'group' AND g.id = t.target_id
	LEFT JOIN models m ON t.target_type = 'model' AND m.id = t.target_id
	LEFT JOIN providers p ON p.id = m.provider_id`

func (s *Service) attachTargets(ctx context.Context, rules []PromptRule) error {
	if len(rules) == 0 {
		return nil
	}
	index := make(map[int64]int, len(rules))
	for i := range rules {
		index[rules[i].ID] = i
	}
	rows, err := s.Store.DB.QueryContext(ctx, promptTargetSelect+` ORDER BY t.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ruleID int64
		var t PromptRuleTarget
		if err := rows.Scan(&ruleID, &t.TargetType, &t.TargetID, &t.Label); err != nil {
			return err
		}
		if i, ok := index[ruleID]; ok {
			rules[i].Targets = append(rules[i].Targets, t)
		}
	}
	return rows.Err()
}

func (s *Service) validatePromptRule(name, prompt string, targets []PromptRuleTarget) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("%w: rule name is required", ErrValidation)
	}
	if utf8.RuneCountInString(name) > maxPromptRuleNameRunes {
		return "", fmt.Errorf("%w: rule name must be at most %d characters", ErrValidation, maxPromptRuleNameRunes)
	}
	if utf8.RuneCountInString(prompt) > maxSystemPromptRunes {
		return "", fmt.Errorf("%w: system prompt must be at most %d characters", ErrValidation, maxSystemPromptRunes)
	}
	if len(targets) == 0 {
		return "", fmt.Errorf("%w: at least one target is required", ErrValidation)
	}
	seen := make(map[string]bool, len(targets))
	for _, t := range targets {
		if t.TargetType != PromptTargetGroup && t.TargetType != PromptTargetModel {
			return "", fmt.Errorf("%w: target type must be 'group' or 'model'", ErrValidation)
		}
		key := t.TargetType + ":" + fmt.Sprint(t.TargetID)
		if seen[key] {
			return "", fmt.Errorf("%w: duplicate target in rule", ErrValidation)
		}
		seen[key] = true
	}
	return name, nil
}

// validateTargetsExist verifies each target references a live group or model.
func (s *Service) validateTargetsExist(ctx context.Context, q queryer, targets []PromptRuleTarget) error {
	for _, t := range targets {
		var exists int
		var err error
		switch t.TargetType {
		case PromptTargetGroup:
			err = q.QueryRowContext(ctx, `SELECT 1 FROM groups WHERE id=?`, t.TargetID).Scan(&exists)
		case PromptTargetModel:
			err = q.QueryRowContext(ctx, `SELECT 1 FROM models WHERE id=?`, t.TargetID).Scan(&exists)
		}
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: target %s %d does not exist", ErrValidation, t.TargetType, t.TargetID)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// validateTargetsClaimable ensures no target is already claimed by another rule.
func (s *Service) validateTargetsClaimable(ctx context.Context, q queryer, targets []PromptRuleTarget, excludeRuleID int64) error {
	for _, t := range targets {
		var owner int64
		err := q.QueryRowContext(ctx,
			`SELECT rule_id FROM prompt_rule_targets WHERE target_type=? AND target_id=?`,
			t.TargetType, t.TargetID).Scan(&owner)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if owner != excludeRuleID {
			return fmt.Errorf("%w: target %s %d is already used by another rule", ErrAlreadyExists, t.TargetType, t.TargetID)
		}
	}
	return nil
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *Service) ListPromptRules(ctx context.Context) ([]PromptRule, error) {
	rows, err := s.Store.DB.QueryContext(ctx, promptRuleSelect+` ORDER BY r.position, r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PromptRule, 0)
	for rows.Next() {
		r, err := scanPromptRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachTargets(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) GetPromptRule(ctx context.Context, id int64) (PromptRule, error) {
	r, err := scanPromptRule(s.Store.DB.QueryRowContext(ctx, promptRuleSelect+` WHERE r.id=?`, id))
	if err != nil {
		return PromptRule{}, err
	}
	rules := []PromptRule{r}
	if err := s.attachTargets(ctx, rules); err != nil {
		return PromptRule{}, err
	}
	return rules[0], nil
}

func (s *Service) CreatePromptRule(ctx context.Context, in PromptRuleInput) (PromptRule, error) {
	name, err := s.validatePromptRule(in.Name, in.Prompt, in.Targets)
	if err != nil {
		return PromptRule{}, err
	}
	if err := s.validateTargetsExist(ctx, s.Store.DB, in.Targets); err != nil {
		return PromptRule{}, err
	}
	if err := s.validateTargetsClaimable(ctx, s.Store.DB, in.Targets, 0); err != nil {
		return PromptRule{}, err
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return PromptRule{}, err
	}
	defer tx.Rollback()
	var maxPos int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) FROM prompt_rules`).Scan(&maxPos); err != nil {
		return PromptRule{}, err
	}
	now := store.Now()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO prompt_rules(name,prompt,inherit,enabled,position,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
		name, in.Prompt, boolInt(in.Inherit), boolInt(in.Enabled), maxPos+1, now, now)
	if err != nil {
		return PromptRule{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return PromptRule{}, err
	}
	if err := insertPromptTargets(ctx, tx, id, in.Targets); err != nil {
		return PromptRule{}, err
	}
	if err := tx.Commit(); err != nil {
		return PromptRule{}, err
	}
	s.invalidatePromptRules()
	return s.GetPromptRule(ctx, id)
}

func (s *Service) UpdatePromptRule(ctx context.Context, id int64, in PromptRuleInput) (PromptRule, error) {
	if _, err := s.GetPromptRule(ctx, id); err != nil {
		return PromptRule{}, err
	}
	name, err := s.validatePromptRule(in.Name, in.Prompt, in.Targets)
	if err != nil {
		return PromptRule{}, err
	}
	if err := s.validateTargetsExist(ctx, s.Store.DB, in.Targets); err != nil {
		return PromptRule{}, err
	}
	if err := s.validateTargetsClaimable(ctx, s.Store.DB, in.Targets, id); err != nil {
		return PromptRule{}, err
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return PromptRule{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`UPDATE prompt_rules SET name=?,prompt=?,inherit=?,enabled=?,updated_at=? WHERE id=?`,
		name, in.Prompt, boolInt(in.Inherit), boolInt(in.Enabled), store.Now(), id)
	if err != nil {
		return PromptRule{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return PromptRule{}, store.ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM prompt_rule_targets WHERE rule_id=?`, id); err != nil {
		return PromptRule{}, err
	}
	if err := insertPromptTargets(ctx, tx, id, in.Targets); err != nil {
		return PromptRule{}, err
	}
	if err := tx.Commit(); err != nil {
		return PromptRule{}, err
	}
	s.invalidatePromptRules()
	return s.GetPromptRule(ctx, id)
}

func insertPromptTargets(ctx context.Context, tx *sql.Tx, ruleID int64, targets []PromptRuleTarget) error {
	for _, t := range targets {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO prompt_rule_targets(rule_id,target_type,target_id) VALUES(?,?,?)`,
			ruleID, t.TargetType, t.TargetID); err != nil {
			if isUnique(err) {
				return fmt.Errorf("%w: target %s %d is already used by another rule", ErrAlreadyExists, t.TargetType, t.TargetID)
			}
			return err
		}
	}
	return nil
}

func (s *Service) DeletePromptRule(ctx context.Context, id int64) error {
	res, err := s.Store.DB.ExecContext(ctx, `DELETE FROM prompt_rules WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	s.invalidatePromptRules()
	return nil
}

func (s *Service) ReorderPromptRules(ctx context.Context, ruleIDs []int64) error {
	rules, err := s.ListPromptRules(ctx)
	if err != nil {
		return err
	}
	if len(ruleIDs) != len(rules) {
		return fmt.Errorf("%w: order must include every rule exactly once", ErrValidation)
	}
	current := make(map[int64]bool, len(rules))
	for _, r := range rules {
		current[r.ID] = true
	}
	seen := make(map[int64]bool, len(ruleIDs))
	for _, id := range ruleIDs {
		if !current[id] || seen[id] {
			return fmt.Errorf("%w: order must include every rule exactly once", ErrValidation)
		}
		seen[id] = true
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := store.Now()
	for position, id := range ruleIDs {
		if _, err := tx.ExecContext(ctx, `UPDATE prompt_rules SET position=?,updated_at=? WHERE id=?`, position, now, id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.invalidatePromptRules()
	return nil
}

// DeletePromptTargetsFor removes target rows pointing at a deleted group or
// model. It is called inside the deleting transaction so orphans never persist.
func deletePromptTargetsFor(ctx context.Context, tx *sql.Tx, targetType string, targetID int64) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM prompt_rule_targets WHERE target_type=? AND target_id=?`, targetType, targetID)
	return err
}

func (s *Service) promptRuleSnapshot(ctx context.Context) []promptRule {
	s.promptMu.RLock()
	if s.promptLoaded {
		rules := s.promptRules
		s.promptMu.RUnlock()
		return rules
	}
	s.promptMu.RUnlock()

	s.promptMu.Lock()
	defer s.promptMu.Unlock()
	if s.promptLoaded {
		return s.promptRules
	}
	rules := s.loadPromptRules(ctx)
	s.promptRules = rules
	s.promptLoaded = true
	return rules
}

func (s *Service) loadPromptRules(ctx context.Context) []promptRule {
	if s.Store == nil || s.Store.DB == nil {
		return nil
	}
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT id,name,prompt,inherit,enabled,position FROM prompt_rules ORDER BY position, id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []promptRule
	for rows.Next() {
		var r promptRule
		var inherit, enabled int
		if err := rows.Scan(&r.ID, &r.Name, &r.Prompt, &inherit, &enabled, &r.Position); err != nil {
			return nil
		}
		r.Inherit = inherit != 0
		r.Enabled = enabled != 0
		r.Groups = map[int64]bool{}
		r.Models = map[int64]bool{}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	index := make(map[int64]int, len(out))
	for i := range out {
		index[out[i].ID] = i
	}
	trows, err := s.Store.DB.QueryContext(ctx, `SELECT rule_id,target_type,target_id FROM prompt_rule_targets`)
	if err != nil {
		return out
	}
	defer trows.Close()
	for trows.Next() {
		var ruleID, targetID int64
		var targetType string
		if err := trows.Scan(&ruleID, &targetType, &targetID); err != nil {
			return out
		}
		i, ok := index[ruleID]
		if !ok {
			continue
		}
		switch targetType {
		case PromptTargetGroup:
			out[i].Groups[targetID] = true
		case PromptTargetModel:
			out[i].Models[targetID] = true
		}
	}
	return out
}

func (s *Service) invalidatePromptRules() {
	s.promptMu.Lock()
	s.promptRules = nil
	s.promptLoaded = false
	s.promptMu.Unlock()
}

// ResolvePromptRules returns the named rule segments that apply to a request,
// in injection order, plus whether the most specific matching rule suppresses
// the global base prompt. groupID is nil for direct (non-group) calls.
func (s *Service) ResolvePromptRules(ctx context.Context, groupID *int64, modelID int64) ([]PromptSegment, bool) {
	rules := s.promptRuleSnapshot(ctx)
	if len(rules) == 0 {
		return nil, false
	}
	var modelRule, groupRule *promptRule
	var segments []PromptSegment
	for i := range rules {
		r := &rules[i]
		if !r.Enabled {
			continue
		}
		matchesModel := modelID != 0 && r.Models[modelID]
		matchesGroup := groupID != nil && r.Groups[*groupID]
		if matchesModel {
			modelRule = r
		}
		if matchesGroup {
			groupRule = r
		}
		if matchesModel || matchesGroup {
			segments = append(segments, PromptSegment{RuleID: r.ID, RuleName: r.Name, Text: r.Prompt})
		}
	}
	suppressed := false
	if modelRule != nil {
		suppressed = !modelRule.Inherit
	} else if groupRule != nil {
		suppressed = !groupRule.Inherit
	}
	return segments, suppressed
}

// PromptRuleByName returns the first enabled rule with the given name
// (case-insensitive), used for the X-Pogu-Prompt override header. The boolean
// reports whether the rule inherits the global base prompt.
func (s *Service) PromptRuleByName(ctx context.Context, name string) (PromptSegment, bool, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return PromptSegment{}, false, false
	}
	rules := s.promptRuleSnapshot(ctx)
	for i := range rules {
		r := &rules[i]
		if r.Enabled && strings.EqualFold(r.Name, name) {
			return PromptSegment{RuleID: r.ID, RuleName: r.Name, Text: r.Prompt}, r.Inherit, true
		}
	}
	return PromptSegment{}, false, false
}
