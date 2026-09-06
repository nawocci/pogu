package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nawocci/pogu/internal/crypto"
	"github.com/nawocci/pogu/internal/store"
)

const providerKeySelect = `SELECT id, provider_id, name, encrypted_secret, enabled, sort_order, last_used_at, created_at, updated_at FROM provider_keys`

func (s *Service) scanProviderKey(row interface{ Scan(...any) error }) (ProviderKey, error) {
	var k ProviderKey
	var encrypted, created, updated string
	var lastUsed sql.NullString
	var enabled int
	err := row.Scan(&k.ID, &k.ProviderID, &k.Name, &encrypted, &enabled, &k.SortOrder, &lastUsed, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return ProviderKey{}, store.ErrNotFound
	}
	if err != nil {
		return ProviderKey{}, err
	}
	k.Enabled = enabled != 0
	k.LastUsedAt = store.NullTime(lastUsed)
	k.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	k.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if secret, decErr := crypto.Decrypt(s.MasterKey, encrypted); decErr == nil {
		k.MaskedKey = crypto.MaskSecret(string(secret))
	}
	return k, nil
}

func (s *Service) nextMonotonicKeyName(ctx context.Context, providerID int64) (string, error) {
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT name FROM provider_keys WHERE provider_id=?`, providerID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	maxN := 0
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			if m := keyNamePattern.FindStringSubmatch(name); len(m) == 2 {
				if n, err := strconv.Atoi(m[1]); err == nil && n > maxN {
					maxN = n
				}
			}
		}
	}
	return fmt.Sprintf("Key %d", maxN+1), nil
}

func (s *Service) CreateProviderKey(ctx context.Context, providerID int64, name, secret string) (ProviderKey, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ProviderKey{}, fmt.Errorf("%w: provider key secret is required", ErrValidation)
	}
	var exists int
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT 1 FROM providers WHERE id=?`, providerID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return ProviderKey{}, store.ErrNotFound
	} else if err != nil {
		return ProviderKey{}, err
	}
	fp := crypto.KeyFingerprint(s.MasterKey, secret)
	var dupID int64
	err := s.Store.DB.QueryRowContext(ctx, `SELECT id FROM provider_keys WHERE provider_id=? AND secret_hash=?`, providerID, fp).Scan(&dupID)
	if err == nil {
		return ProviderKey{}, fmt.Errorf("%w: duplicate provider key secret", ErrAlreadyExists)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ProviderKey{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		var err error
		name, err = s.nextMonotonicKeyName(ctx, providerID)
		if err != nil {
			return ProviderKey{}, err
		}
	}
	encSecret, err := crypto.Encrypt(s.MasterKey, []byte(secret))
	if err != nil {
		return ProviderKey{}, fmt.Errorf("encrypt provider secret: %w", err)
	}
	var maxSort int
	_ = s.Store.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), -1) FROM provider_keys WHERE provider_id=?`, providerID).Scan(&maxSort)
	now := store.Now()
	res, err := s.Store.DB.ExecContext(ctx,
		`INSERT INTO provider_keys(provider_id, name, encrypted_secret, secret_hash, enabled, sort_order, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?)`,
		providerID, name, encSecret, fp, 1, maxSort+1, now, now)
	if err != nil {
		if isUnique(err) {
			return ProviderKey{}, fmt.Errorf("%w: duplicate provider key secret", ErrAlreadyExists)
		}
		return ProviderKey{}, err
	}
	keyID, err := res.LastInsertId()
	if err != nil {
		return ProviderKey{}, err
	}
	return s.GetProviderKey(ctx, keyID)
}

func (s *Service) GetProviderKey(ctx context.Context, id int64) (ProviderKey, error) {
	return s.scanProviderKey(s.Store.DB.QueryRowContext(ctx, providerKeySelect+` WHERE id=?`, id))
}

func (s *Service) ListProviderKeys(ctx context.Context, providerID int64) ([]ProviderKey, error) {
	rows, err := s.Store.DB.QueryContext(ctx, providerKeySelect+` WHERE provider_id=? ORDER BY sort_order ASC, id ASC`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProviderKey
	for rows.Next() {
		k, err := s.scanProviderKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Service) UpdateProviderKey(ctx context.Context, id int64, name string, enabled bool) (ProviderKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ProviderKey{}, fmt.Errorf("%w: key name cannot be empty", ErrValidation)
	}
	now := store.Now()
	res, err := s.Store.DB.ExecContext(ctx, `UPDATE provider_keys SET name=?, enabled=?, updated_at=? WHERE id=?`, name, boolInt(enabled), now, id)
	if err != nil {
		return ProviderKey{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ProviderKey{}, store.ErrNotFound
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

func (s *Service) SetProviderKeyPrimary(ctx context.Context, providerID, keyID int64) error {
	keys, err := s.ListProviderKeys(ctx, providerID)
	if err != nil {
		return err
	}
	targetIdx := -1
	for i, k := range keys {
		if k.ID == keyID {
			targetIdx = i
			break
		}
	}
	if targetIdx == -1 {
		return store.ErrNotFound
	}
	if targetIdx == 0 {
		return nil
	}
	target := keys[targetIdx]
	reordered := make([]ProviderKey, 0, len(keys))
	reordered = append(reordered, target)
	for i, k := range keys {
		if i != targetIdx {
			reordered = append(reordered, k)
		}
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := store.Now()
	for i, k := range reordered {
		if _, err := tx.ExecContext(ctx, `UPDATE provider_keys SET sort_order=?, updated_at=? WHERE id=?`, i, now, k.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) ProviderSecret(ctx context.Context, p Provider) (string, error) {
	return s.ProviderPrimarySecret(ctx, p.ID)
}

func (s *Service) ProviderKeySecret(ctx context.Context, keyID int64) (string, error) {
	var encrypted string
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT encrypted_secret FROM provider_keys WHERE id=?`, keyID).Scan(&encrypted); err != nil {
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
	var encrypted string
	err := s.Store.DB.QueryRowContext(ctx, `SELECT encrypted_secret FROM provider_keys WHERE provider_id=? AND enabled=1 ORDER BY sort_order ASC, id ASC LIMIT 1`, providerID).Scan(&encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: no enabled provider keys", store.ErrNotFound)
	}
	if err != nil {
		return "", err
	}
	plain, err := crypto.Decrypt(s.MasterKey, encrypted)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (s *Service) TouchProviderKeyUsed(ctx context.Context, keyID int64) error {
	_, err := s.Store.DB.ExecContext(ctx, `UPDATE provider_keys SET last_used_at=? WHERE id=?`, store.Now(), keyID)
	return err
}

func (s *Service) EligibleKeys(ctx context.Context, providerID int64) ([]ProviderKey, error) {
	p, err := s.GetProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	keys, err := s.ListProviderKeys(ctx, providerID)
	if err != nil {
		return nil, err
	}
	var enabled []ProviderKey
	for _, k := range keys {
		if k.Enabled {
			enabled = append(enabled, k)
		}
	}
	if len(enabled) == 0 {
		return nil, nil
	}
	if p.KeySelection == KeySelectionRoundRobin && len(enabled) > 1 {
		cursor := s.nextRoundRobinCursor("provider", providerID, len(enabled))
		ordered := make([]ProviderKey, len(enabled))
		for i := range enabled {
			ordered[i] = enabled[(cursor+i)%len(enabled)]
		}
		return ordered, nil
	}
	return enabled, nil
}
