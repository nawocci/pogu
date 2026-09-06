package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nawocci/pogu/internal/crypto"
	"github.com/nawocci/pogu/internal/store"
)

func (s *Service) CreateAPIKey(ctx context.Context, name string) (APIKey, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 200 {
		return APIKey{}, "", fmt.Errorf("%w: key name is required and must be at most 200 characters", ErrValidation)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return APIKey{}, "", fmt.Errorf("generate API key: %w", err)
	}
	plaintext := "sk-pogu-" + base64.RawURLEncoding.EncodeToString(raw)
	hash := crypto.HashToken(plaintext)
	now := store.Now()
	result, err := s.Store.DB.ExecContext(ctx, `INSERT INTO client_keys(name,key_hash,created_at) VALUES(?,?,?)`, name, hash, now)
	if err != nil {
		return APIKey{}, "", err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return APIKey{}, "", err
	}
	key, err := s.GetAPIKey(ctx, id)
	return key, plaintext, err
}

func scanAPIKey(row interface{ Scan(...any) error }) (APIKey, error) {
	var k APIKey
	var last, expires, revoked sql.NullString
	var created string
	err := row.Scan(&k.ID, &k.Name, &last, &expires, &revoked, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return APIKey{}, store.ErrNotFound
	}
	if err != nil {
		return APIKey{}, err
	}
	k.LastUsedAt = store.NullTime(last)
	k.ExpiresAt = store.NullTime(expires)
	k.RevokedAt = store.NullTime(revoked)
	k.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return k, nil
}

func (s *Service) GetAPIKey(ctx context.Context, id int64) (APIKey, error) {
	return scanAPIKey(s.Store.DB.QueryRowContext(ctx, `SELECT id,name,last_used_at,expires_at,revoked_at,created_at FROM client_keys WHERE id=?`, id))
}

func (s *Service) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT id,name,last_used_at,expires_at,revoked_at,created_at FROM client_keys ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Service) RevokeAPIKey(ctx context.Context, id int64) error {
	result, err := s.Store.DB.ExecContext(ctx, `UPDATE client_keys SET revoked_at=COALESCE(revoked_at,?) WHERE id=?`, store.Now(), id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Service) AuthenticateAPIKey(ctx context.Context, plaintext string) (APIKey, error) {
	if !strings.HasPrefix(plaintext, "sk-pogu-") || len(plaintext) < len("sk-pogu-")+20 {
		return APIKey{}, ErrUnauthorized
	}
	hash := crypto.HashToken(plaintext)
	var id int64
	err := s.Store.DB.QueryRowContext(ctx, `SELECT id FROM client_keys WHERE key_hash=? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>?)`, hash, store.Now()).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return APIKey{}, ErrUnauthorized
	}
	if err != nil {
		return APIKey{}, err
	}
	if _, err := s.Store.DB.ExecContext(ctx, `UPDATE client_keys SET last_used_at=? WHERE id=?`, store.Now(), id); err != nil {
		return APIKey{}, err
	}
	return s.GetAPIKey(ctx, id)
}

func (s *Service) CreateSession(ctx context.Context, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now().UTC()
	_, _ = s.Store.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at<=?`, store.Now())
	_, err := s.Store.DB.ExecContext(ctx, `INSERT INTO sessions(token_hash,created_at,expires_at) VALUES(?,?,?)`, crypto.HashToken(token), now.Format(time.RFC3339Nano), now.Add(ttl).Format(time.RFC3339Nano))
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) ValidateSession(ctx context.Context, token string) bool {
	if token == "" {
		return false
	}
	var expires string
	err := s.Store.DB.QueryRowContext(ctx, `SELECT expires_at FROM sessions WHERE token_hash=?`, crypto.HashToken(token)).Scan(&expires)
	if err != nil {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || !time.Now().UTC().Before(expiresAt) {
		_, _ = s.Store.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, crypto.HashToken(token))
		return false
	}
	return true
}

func (s *Service) DeleteSession(ctx context.Context, token string) error {
	_, err := s.Store.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, crypto.HashToken(token))
	return err
}
