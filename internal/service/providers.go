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

const providerSelect = `SELECT p.id, p.name, p.type, p.prefix, p.base_url, p.key_selection, p.enabled, p.created_at, p.updated_at, (SELECT COUNT(1) FROM provider_keys pk WHERE pk.provider_id=p.id), p.builtin FROM providers p`

func scanProvider(row interface{ Scan(...any) error }) (Provider, error) {
	var p Provider
	var typ, keySel string
	var enabled, keyCount int
	var created, updated string
	err := row.Scan(&p.ID, &p.Name, &typ, &p.Prefix, &p.BaseURL, &keySel, &enabled, &created, &updated, &keyCount, &p.Builtin)
	if errors.Is(err, sql.ErrNoRows) {
		return Provider{}, store.ErrNotFound
	}
	if err != nil {
		return Provider{}, err
	}
	p.Type = ProviderType(typ)
	p.KeySelection = KeySelection(keySel)
	if !p.KeySelection.Valid() {
		p.KeySelection = KeySelectionFirst
	}
	p.KeyCount = keyCount
	p.HasAPIKey = keyCount > 0
	p.Enabled = enabled != 0
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return p, nil
}

func (s *Service) CreateProvider(ctx context.Context, name string, typ ProviderType, prefix, baseURL, apiKey string, enabled bool, keySelection ...KeySelection) (Provider, error) {
	name = strings.TrimSpace(name)
	prefix = strings.TrimSpace(prefix)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if err := validateProviderInput(name, typ, prefix, baseURL); err != nil {
		return Provider{}, err
	}
	ks := KeySelectionFirst
	if len(keySelection) > 0 && keySelection[0] != "" {
		if !keySelection[0].Valid() {
			return Provider{}, fmt.Errorf("%w: key_selection must be 'first' or 'round_robin'", ErrValidation)
		}
		ks = keySelection[0]
	}
	now := store.Now()
	result, err := s.Store.DB.ExecContext(ctx, `INSERT INTO providers(name,type,prefix,base_url,key_selection,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, name, typ, prefix, baseURL, ks, boolInt(enabled), now, now)
	if err != nil {
		if isUnique(err) {
			return Provider{}, fmt.Errorf("%w: prefix already exists", ErrAlreadyExists)
		}
		return Provider{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Provider{}, err
	}
	if strings.TrimSpace(apiKey) != "" {
		if _, err := s.CreateProviderKey(ctx, id, "", apiKey); err != nil {
			return Provider{}, err
		}
	}
	return s.GetProvider(ctx, id)
}

func (s *Service) GetProvider(ctx context.Context, id int64) (Provider, error) {
	return scanProvider(s.Store.DB.QueryRowContext(ctx, providerSelect+` WHERE p.id=?`, id))
}

func (s *Service) ListProviders(ctx context.Context) ([]Provider, error) {
	rows, err := s.Store.DB.QueryContext(ctx, providerSelect+` ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Provider
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Service) UpdateProvider(ctx context.Context, id int64, name string, typ ProviderType, prefix, baseURL string, enabled bool, keySelection ...KeySelection) (Provider, error) {
	name = strings.TrimSpace(name)
	prefix = strings.TrimSpace(prefix)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	existing, err := s.GetProvider(ctx, id)
	if err != nil {
		return Provider{}, err
	}
	if existing.Builtin != "" {
		if prefix != existing.Prefix {
			return Provider{}, fmt.Errorf("%w: built-in provider prefix cannot be changed", ErrBuiltin)
		}
		if err := validateProviderNameTypeURL(name, typ, baseURL); err != nil {
			return Provider{}, err
		}
	} else if err := validateProviderInput(name, typ, prefix, baseURL); err != nil {
		return Provider{}, err
	}
	ks := KeySelectionFirst
	if len(keySelection) > 0 && keySelection[0] != "" {
		if !keySelection[0].Valid() {
			return Provider{}, fmt.Errorf("%w: key_selection must be 'first' or 'round_robin'", ErrValidation)
		}
		ks = keySelection[0]
	} else {
		var currentKs string
		if err := s.Store.DB.QueryRowContext(ctx, `SELECT key_selection FROM providers WHERE id=?`, id).Scan(&currentKs); err == nil {
			ks = KeySelection(currentKs)
		}
	}
	now := store.Now()
	result, err := s.Store.DB.ExecContext(ctx, `UPDATE providers SET name=?,type=?,prefix=?,base_url=?,key_selection=?,enabled=?,updated_at=? WHERE id=?`, name, typ, prefix, baseURL, ks, boolInt(enabled), now, id)
	if err != nil {
		if isUnique(err) {
			return Provider{}, fmt.Errorf("%w: prefix already exists", ErrAlreadyExists)
		}
		return Provider{}, err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return Provider{}, store.ErrNotFound
	}
	return s.GetProvider(ctx, id)
}

func (s *Service) DeleteProvider(ctx context.Context, id int64) error {
	p, err := s.GetProvider(ctx, id)
	if err != nil {
		return err
	}
	if p.Builtin != "" {
		return fmt.Errorf("%w: disable it instead", ErrBuiltin)
	}
	result, err := s.Store.DB.ExecContext(ctx, `DELETE FROM providers WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}
