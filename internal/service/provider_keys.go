package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/nawocci/pogu/internal/crypto"
	"github.com/nawocci/pogu/internal/store"
)

func (s *Service) CreateProviderKey(ctx context.Context, providerID int64, name, secret string) (ProviderKey, string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ProviderKey{}, "", validation("key value is required")
	}
	return s.createProviderKey(ctx, providerID, name, secret)
}

func (s *Service) createProviderKey(ctx context.Context, providerID int64, name, secret string) (ProviderKey, string, error) {
	if _, err := s.GetProvider(ctx, providerID); err != nil {
		return ProviderKey{}, "", err
	}
	name = strings.TrimSpace(name)
	if name != "" {
		if _, err := checkName(name); err != nil {
			return ProviderKey{}, "", err
		}
	}
	fingerprint := crypto.KeyFingerprint(s.MasterKey, secret)
	var exists int
	err := s.Store.DB.QueryRowContext(ctx,
		`SELECT 1 FROM provider_keys WHERE provider_id=? AND secret_hash=?`, providerID, fingerprint).Scan(&exists)
	if err == nil {
		return ProviderKey{}, "", &ValidationError{"this key is already stored for the provider"}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ProviderKey{}, "", err
	}
	if name == "" {
		name, err = s.nextKeyName(ctx, providerID)
		if err != nil {
			return ProviderKey{}, "", err
		}
	}
	encrypted, err := crypto.Encrypt(s.MasterKey, []byte(secret))
	if err != nil {
		return ProviderKey{}, "", err
	}
	var maxOrder sql.NullInt64
	_ = s.Store.DB.QueryRowContext(ctx,
		`SELECT MAX(sort_order) FROM provider_keys WHERE provider_id=?`, providerID).Scan(&maxOrder)
	order := 0
	if maxOrder.Valid {
		order = int(maxOrder.Int64) + 1
	}
	now := store.Now()
	res, err := s.Store.DB.ExecContext(ctx,
		`INSERT INTO provider_keys(provider_id, name, encrypted_secret, secret_hash, enabled, sort_order, created_at, updated_at)
		 VALUES(?, ?, ?, ?, 1, ?, ?, ?)`,
		providerID, name, encrypted, fingerprint, order, now, now)
	if err != nil {
		if isUnique(err) {
			return ProviderKey{}, "", &ValidationError{"this key is already stored for the provider"}
		}
		return ProviderKey{}, "", err
	}
	id, _ := res.LastInsertId()
	key, err := s.GetProviderKey(ctx, id)
	if err != nil {
		return ProviderKey{}, "", err
	}
	return key, secret, nil
}

func (s *Service) nextKeyName(ctx context.Context, providerID int64) (string, error) {
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT name FROM provider_keys WHERE provider_id=?`, providerID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", err
		}
		if m := keyNamePattern.FindStringSubmatch(name); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > max {
				max = n
			}
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return fmt.Sprintf("Key %d", max+1), nil
}

func (s *Service) GetProviderKey(ctx context.Context, id int64) (ProviderKey, error) {
	var k ProviderKey
	var enabled int
	var lastUsed sql.NullString
	var encrypted string
	err := s.Store.DB.QueryRowContext(ctx, `
		SELECT id, provider_id, name, encrypted_secret, enabled, sort_order, last_used_at, created_at, updated_at
		FROM provider_keys WHERE id=?`, id).Scan(
		&k.ID, &k.ProviderID, &k.Name, &encrypted, &enabled, &k.SortOrder, &lastUsed, &k.CreatedAt, &k.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProviderKey{}, store.ErrNotFound
		}
		return ProviderKey{}, err
	}
	k.Enabled = enabled == 1
	if lastUsed.Valid {
		v := lastUsed.String
		k.LastUsedAt = &v
	}
	if plain, err := crypto.Decrypt(s.MasterKey, encrypted); err == nil {
		k.MaskedKey = crypto.MaskSecret(string(plain))
	}
	return k, nil
}

func (s *Service) ListProviderKeys(ctx context.Context, providerID int64) ([]ProviderKey, error) {
	if _, err := s.GetProvider(ctx, providerID); err != nil {
		return nil, err
	}
	rows, err := s.Store.DB.QueryContext(ctx, `
		SELECT id, provider_id, name, encrypted_secret, enabled, sort_order, last_used_at, created_at, updated_at
		FROM provider_keys WHERE provider_id=? ORDER BY sort_order, id`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProviderKey{}
	for rows.Next() {
		var k ProviderKey
		var enabled int
		var lastUsed sql.NullString
		var encrypted string
		if err := rows.Scan(&k.ID, &k.ProviderID, &k.Name, &encrypted, &enabled, &k.SortOrder,
			&lastUsed, &k.CreatedAt, &k.UpdatedAt); err != nil {
			return nil, err
		}
		k.Enabled = enabled == 1
		if lastUsed.Valid {
			v := lastUsed.String
			k.LastUsedAt = &v
		}
		if plain, err := crypto.Decrypt(s.MasterKey, encrypted); err == nil {
			k.MaskedKey = crypto.MaskSecret(string(plain))
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Service) UpdateProviderKey(ctx context.Context, id int64, name string, enabled *bool) (ProviderKey, error) {
	cur, err := s.GetProviderKey(ctx, id)
	if err != nil {
		return ProviderKey{}, err
	}
	if name != "" {
		if name, err = checkName(name); err != nil {
			return ProviderKey{}, err
		}
	} else {
		name = cur.Name
	}
	isEnabled := cur.Enabled
	if enabled != nil {
		isEnabled = *enabled
	}
	_, err = s.Store.DB.ExecContext(ctx,
		`UPDATE provider_keys SET name=?, enabled=?, updated_at=? WHERE id=?`,
		name, boolInt(isEnabled), store.Now(), id)
	if err != nil {
		return ProviderKey{}, err
	}
	return s.GetProviderKey(ctx, id)
}

func (s *Service) DeleteProviderKey(ctx context.Context, id int64) error {
	res, err := s.Store.DB.ExecContext(ctx, `DELETE FROM provider_keys WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Service) SetProviderKeyPrimary(ctx context.Context, id int64) ([]ProviderKey, error) {
	cur, err := s.GetProviderKey(ctx, id)
	if err != nil {
		return nil, err
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`UPDATE provider_keys SET sort_order = sort_order + 1 WHERE provider_id=?`, cur.ProviderID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE provider_keys SET sort_order = 0, updated_at=? WHERE id=?`, store.Now(), id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListProviderKeys(ctx, cur.ProviderID)
}

func (s *Service) EligibleKeys(ctx context.Context, providerID int64) ([]ProviderKey, error) {
	all, err := s.ListProviderKeys(ctx, providerID)
	if err != nil {
		return nil, err
	}
	out := make([]ProviderKey, 0, len(all))
	for _, k := range all {
		if k.Enabled {
			out = append(out, k)
		}
	}
	return out, nil
}

func (s *Service) ProviderKeySecret(ctx context.Context, id int64) (string, error) {
	var encrypted string
	err := s.Store.DB.QueryRowContext(ctx, `SELECT encrypted_secret FROM provider_keys WHERE id=?`, id).Scan(&encrypted)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", store.ErrNotFound
		}
		return "", err
	}
	plain, err := crypto.Decrypt(s.MasterKey, encrypted)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (s *Service) ProviderPrimarySecret(ctx context.Context, providerID int64) (string, error) {
	keys, err := s.EligibleKeys(ctx, providerID)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", store.ErrNotFound
	}
	return s.ProviderKeySecret(ctx, keys[0].ID)
}

func (s *Service) TouchProviderKeyUsed(ctx context.Context, id int64) {
	_, _ = s.Store.DB.ExecContext(ctx, `UPDATE provider_keys SET last_used_at=? WHERE id=?`, store.Now(), id)
}
