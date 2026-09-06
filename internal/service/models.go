package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nawocci/pogu/internal/store"
)

const modelSelect = `SELECT m.id,m.provider_id,p.name,p.prefix,m.name,m.enabled,m.created_at,m.updated_at,m.scheme FROM models m JOIN providers p ON p.id=m.provider_id`

func scanModel(row interface{ Scan(...any) error }) (Model, error) {
	var m Model
	var enabled int
	var created, updated string
	err := row.Scan(&m.ID, &m.ProviderID, &m.Provider, &m.Prefix, &m.Name, &enabled, &created, &updated, &m.Scheme)
	if errors.Is(err, sql.ErrNoRows) {
		return Model{}, store.ErrNotFound
	}
	if err != nil {
		return Model{}, err
	}
	m.PublicID = m.Prefix + "/" + m.Name
	m.Enabled = enabled != 0
	m.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	m.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return m, nil
}

func (s *Service) CreateModel(ctx context.Context, providerID int64, name string, enabled bool) (Model, error) {
	if err := validateModelName(name); err != nil {
		return Model{}, err
	}
	var exists int
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT 1 FROM providers WHERE id=?`, providerID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return Model{}, store.ErrNotFound
	} else if err != nil {
		return Model{}, err
	}
	now := store.Now()
	result, err := s.Store.DB.ExecContext(ctx, `INSERT INTO models(provider_id,name,enabled,created_at,updated_at) VALUES(?,?,?,?,?)`, providerID, name, boolInt(enabled), now, now)
	if err != nil {
		if isUnique(err) {
			return Model{}, fmt.Errorf("%w: model already exists", ErrAlreadyExists)
		}
		return Model{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Model{}, err
	}
	return s.GetModel(ctx, id)
}

func (s *Service) GetModel(ctx context.Context, id int64) (Model, error) {
	return scanModel(s.Store.DB.QueryRowContext(ctx, modelSelect+` WHERE m.id=?`, id))
}

func (s *Service) ListModels(ctx context.Context) ([]Model, error) {
	rows, err := s.Store.DB.QueryContext(ctx, modelSelect+` ORDER BY m.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Model
	for rows.Next() {
		m, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Service) UpdateModel(ctx context.Context, id int64, name string, enabled bool) (Model, error) {
	if err := validateModelName(name); err != nil {
		return Model{}, err
	}
	result, err := s.Store.DB.ExecContext(ctx, `UPDATE models SET name=?,enabled=?,updated_at=? WHERE id=?`, name, boolInt(enabled), store.Now(), id)
	if err != nil {
		if isUnique(err) {
			return Model{}, fmt.Errorf("%w: model already exists", ErrAlreadyExists)
		}
		return Model{}, err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return Model{}, store.ErrNotFound
	}
	return s.GetModel(ctx, id)
}

func (s *Service) DeleteModel(ctx context.Context, id int64) error {
	result, err := s.Store.DB.ExecContext(ctx, `DELETE FROM models WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Service) SetModelEnabled(ctx context.Context, id int64, enabled bool) error {
	result, err := s.Store.DB.ExecContext(ctx, `UPDATE models SET enabled=?,updated_at=? WHERE id=?`, boolInt(enabled), store.Now(), id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Service) SetModelScheme(ctx context.Context, id int64, scheme string) error {
	if !ValidModelScheme(scheme) {
		return fmt.Errorf("%w: unknown model scheme %q", ErrValidation, scheme)
	}
	result, err := s.Store.DB.ExecContext(ctx, `UPDATE models SET scheme=?,updated_at=? WHERE id=?`, scheme, store.Now(), id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Service) ResolveRoute(ctx context.Context, publicID string) (Route, error) {
	parts := strings.Split(publicID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Route{}, fmt.Errorf("%w: malformed model identifier", ErrUnknownRoute)
	}
	m, err := scanModel(s.Store.DB.QueryRowContext(ctx, modelSelect+` WHERE p.prefix=? AND m.name=? AND p.enabled=1 AND m.enabled=1`, parts[0], parts[1]))
	if errors.Is(err, store.ErrNotFound) {
		return Route{}, ErrUnknownRoute
	}
	if err != nil {
		return Route{}, err
	}
	p, err := s.GetProvider(ctx, m.ProviderID)
	if err != nil {
		return Route{}, err
	}
	return Route{Provider: p, Model: m}, nil
}
