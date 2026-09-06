package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/nawocci/pogu/internal/store"
)

func (s *Service) CreateProvider(ctx context.Context, name string, typ ProviderType, prefix, baseURL, apiKey string, enabled bool, selection KeySelection) (Provider, error) {
	name, err := checkName(name)
	if err != nil {
		return Provider{}, err
	}
	if err := checkProviderType(typ); err != nil {
		return Provider{}, err
	}
	prefix, err = checkPrefix(prefix)
	if err != nil {
		return Provider{}, err
	}
	baseURL, err = checkBaseURL(baseURL)
	if err != nil {
		return Provider{}, err
	}
	selection, err = checkSelection(selection)
	if err != nil {
		return Provider{}, err
	}
	now := store.Now()
	res, err := s.Store.DB.ExecContext(ctx,
		`INSERT INTO providers(name, type, prefix, base_url, key_selection, enabled, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		name, string(typ), prefix, baseURL, string(selection), boolInt(enabled), now, now)
	if err != nil {
		if isUnique(err) {
			return Provider{}, &ValidationError{"prefix already exists"}
		}
		return Provider{}, err
	}
	id, _ := res.LastInsertId()
	if strings.TrimSpace(apiKey) != "" {
		if _, _, err := s.createProviderKey(ctx, id, "", strings.TrimSpace(apiKey)); err != nil {
			return Provider{}, err
		}
	}
	return s.GetProvider(ctx, id)
}

func (s *Service) GetProvider(ctx context.Context, id int64) (Provider, error) {
	var p Provider
	var enabled int
	var keyCount int
	err := s.Store.DB.QueryRowContext(ctx, `
		SELECT p.id, p.name, p.type, p.prefix, p.base_url, p.key_selection, p.enabled, p.builtin,
		       p.created_at, p.updated_at, COUNT(k.id)
		FROM providers p LEFT JOIN provider_keys k ON k.provider_id = p.id
		WHERE p.id = ? GROUP BY p.id`, id).Scan(
		&p.ID, &p.Name, &p.Type, &p.Prefix, &p.BaseURL, &p.KeySelection, &enabled, &p.Builtin,
		&p.CreatedAt, &p.UpdatedAt, &keyCount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Provider{}, store.ErrNotFound
		}
		return Provider{}, err
	}
	p.Enabled = enabled == 1
	p.KeyCount = keyCount
	p.HasAPIKey = keyCount > 0
	return p, nil
}

func (s *Service) ListProviders(ctx context.Context) ([]Provider, error) {
	rows, err := s.Store.DB.QueryContext(ctx, `
		SELECT p.id, p.name, p.type, p.prefix, p.base_url, p.key_selection, p.enabled, p.builtin,
		       p.created_at, p.updated_at, COUNT(k.id)
		FROM providers p LEFT JOIN provider_keys k ON k.provider_id = p.id
		GROUP BY p.id ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Provider{}
	for rows.Next() {
		var p Provider
		var enabled, keyCount int
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.Prefix, &p.BaseURL, &p.KeySelection,
			&enabled, &p.Builtin, &p.CreatedAt, &p.UpdatedAt, &keyCount); err != nil {
			return nil, err
		}
		p.Enabled = enabled == 1
		p.KeyCount = keyCount
		p.HasAPIKey = keyCount > 0
		out = append(out, p)
	}
	return out, rows.Err()
}

type ProviderUpdate struct {
	Name         string
	Type         ProviderType
	Prefix       string
	BaseURL      string
	KeySelection KeySelection
	Enabled      *bool
	SetType      bool
	SetSelection bool
}

func (s *Service) UpdateProvider(ctx context.Context, id int64, up ProviderUpdate) (Provider, error) {
	cur, err := s.GetProvider(ctx, id)
	if err != nil {
		return Provider{}, err
	}
	if up.Prefix != "" && strings.TrimSpace(up.Prefix) != cur.Prefix && cur.Builtin != "" {
		return Provider{}, ErrBuiltin
	}
	name := cur.Name
	if up.Name != "" {
		if name, err = checkName(up.Name); err != nil {
			return Provider{}, err
		}
	}
	typ := cur.Type
	if up.SetType {
		if err := checkProviderType(up.Type); err != nil {
			return Provider{}, err
		}
		typ = up.Type
	}
	prefix := cur.Prefix
	if up.Prefix != "" {
		if prefix, err = checkPrefix(up.Prefix); err != nil {
			return Provider{}, err
		}
	}
	baseURL := cur.BaseURL
	if up.BaseURL != "" {
		if baseURL, err = checkBaseURL(up.BaseURL); err != nil {
			return Provider{}, err
		}
	}
	selection := cur.KeySelection
	if up.SetSelection {
		if selection, err = checkSelection(up.KeySelection); err != nil {
			return Provider{}, err
		}
	}
	enabled := cur.Enabled
	if up.Enabled != nil {
		enabled = *up.Enabled
	}
	_, err = s.Store.DB.ExecContext(ctx,
		`UPDATE providers SET name=?, type=?, prefix=?, base_url=?, key_selection=?, enabled=?, updated_at=? WHERE id=?`,
		name, string(typ), prefix, baseURL, string(selection), boolInt(enabled), store.Now(), id)
	if err != nil {
		if isUnique(err) {
			return Provider{}, &ValidationError{"prefix already exists"}
		}
		return Provider{}, err
	}
	return s.GetProvider(ctx, id)
}

func (s *Service) DeleteProvider(ctx context.Context, id int64) error {
	cur, err := s.GetProvider(ctx, id)
	if err != nil {
		return err
	}
	if cur.Builtin != "" {
		return ErrBuiltin
	}
	res, err := s.Store.DB.ExecContext(ctx, `DELETE FROM providers WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func isUnique(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed")
}
