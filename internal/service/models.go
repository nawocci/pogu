package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/nawocci/pogu/internal/store"
)

func (s *Service) CreateModel(ctx context.Context, providerID int64, name string, enabled bool) (Model, error) {
	name, err := checkModelName(name)
	if err != nil {
		return Model{}, err
	}
	if _, err := s.GetProvider(ctx, providerID); err != nil {
		return Model{}, err
	}
	now := store.Now()
	res, err := s.Store.DB.ExecContext(ctx,
		`INSERT INTO models(provider_id, name, enabled, created_at, updated_at) VALUES(?, ?, ?, ?, ?)`,
		providerID, name, boolInt(enabled), now, now)
	if err != nil {
		if isUnique(err) {
			return Model{}, &ValidationError{"model already exists for this provider"}
		}
		return Model{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetModel(ctx, id)
}

func (s *Service) GetModel(ctx context.Context, id int64) (Model, error) {
	var m Model
	var enabled int
	err := s.Store.DB.QueryRowContext(ctx, `
		SELECT m.id, m.provider_id, p.name, p.prefix, m.name, m.scheme, m.enabled, m.created_at, m.updated_at
		FROM models m JOIN providers p ON p.id = m.provider_id WHERE m.id=?`, id).Scan(
		&m.ID, &m.ProviderID, &m.Provider, &m.Prefix, &m.Name, &m.Scheme, &enabled, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Model{}, store.ErrNotFound
		}
		return Model{}, err
	}
	m.Enabled = enabled == 1
	m.PublicID = m.Prefix + "/" + m.Name
	return m, nil
}

func (s *Service) ListModels(ctx context.Context) ([]Model, error) {
	rows, err := s.Store.DB.QueryContext(ctx, `
		SELECT m.id, m.provider_id, p.name, p.prefix, m.name, m.scheme, m.enabled, m.created_at, m.updated_at
		FROM models m JOIN providers p ON p.id = m.provider_id ORDER BY p.prefix, m.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Model{}
	for rows.Next() {
		var m Model
		var enabled int
		if err := rows.Scan(&m.ID, &m.ProviderID, &m.Provider, &m.Prefix, &m.Name, &m.Scheme,
			&enabled, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		m.Enabled = enabled == 1
		m.PublicID = m.Prefix + "/" + m.Name
		out = append(out, m)
	}
	return out, rows.Err()
}

type ModelUpdate struct {
	Name    string
	Scheme  *string
	Enabled *bool
}

func (s *Service) UpdateModel(ctx context.Context, id int64, up ModelUpdate) (Model, error) {
	cur, err := s.GetModel(ctx, id)
	if err != nil {
		return Model{}, err
	}
	name := cur.Name
	if up.Name != "" {
		if name, err = checkModelName(up.Name); err != nil {
			return Model{}, err
		}
	}
	scheme := cur.Scheme
	if up.Scheme != nil {
		if scheme, err = checkScheme(*up.Scheme); err != nil {
			return Model{}, err
		}
	}
	enabled := cur.Enabled
	if up.Enabled != nil {
		enabled = *up.Enabled
	}
	_, err = s.Store.DB.ExecContext(ctx,
		`UPDATE models SET name=?, scheme=?, enabled=?, updated_at=? WHERE id=?`,
		name, scheme, boolInt(enabled), store.Now(), id)
	if err != nil {
		if isUnique(err) {
			return Model{}, &ValidationError{"model already exists for this provider"}
		}
		return Model{}, err
	}
	return s.GetModel(ctx, id)
}

func (s *Service) DeleteModel(ctx context.Context, id int64) error {
	res, err := s.Store.DB.ExecContext(ctx, `DELETE FROM models WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Service) ResolveRoute(ctx context.Context, publicID string) (Route, error) {
	parts := strings.Split(publicID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Route{}, &ValidationError{"model must be prefix/model"}
	}
	var m Model
	var p Provider
	var modelEnabled, providerEnabled int
	err := s.Store.DB.QueryRowContext(ctx, `
		SELECT m.id, m.provider_id, p.name, p.prefix, m.name, m.scheme, m.enabled,
		       p.id, p.name, p.type, p.prefix, p.base_url, p.key_selection, p.enabled, p.builtin,
		       p.created_at, p.updated_at
		FROM models m JOIN providers p ON p.id = m.provider_id
		WHERE p.prefix=? AND m.name=?`, parts[0], parts[1]).Scan(
		&m.ID, &m.ProviderID, &m.Provider, &m.Prefix, &m.Name, &m.Scheme, &modelEnabled,
		&p.ID, &p.Name, &p.Type, &p.Prefix, &p.BaseURL, &p.KeySelection, &providerEnabled, &p.Builtin,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Route{}, ErrUnknownRoute
		}
		return Route{}, err
	}
	if modelEnabled != 1 || providerEnabled != 1 {
		return Route{}, ErrUnknownRoute
	}
	p.Enabled = true
	m.Enabled = true
	m.PublicID = m.Prefix + "/" + m.Name
	return Route{Provider: p, Model: m}, nil
}
