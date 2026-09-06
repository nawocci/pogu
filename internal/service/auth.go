package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nawocci/pogu/internal/crypto"
	"github.com/nawocci/pogu/internal/store"
)

const clientKeyPrefix = "sk-pogu-"

func (s *Service) CreateAPIKey(ctx context.Context, name string) (APIKey, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 200 {
		return APIKey{}, "", validation("name must be between 1 and 200 characters")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return APIKey{}, "", err
	}
	secret := clientKeyPrefix + base64.RawURLEncoding.EncodeToString(raw)
	now := store.Now()
	res, err := s.Store.DB.ExecContext(ctx,
		`INSERT INTO client_keys(name, key_hash, created_at) VALUES(?, ?, ?)`,
		name, crypto.HashToken(secret), now)
	if err != nil {
		return APIKey{}, "", err
	}
	id, _ := res.LastInsertId()
	key, err := s.getAPIKey(ctx, id)
	if err != nil {
		return APIKey{}, "", err
	}
	return key, secret, nil
}

func (s *Service) getAPIKey(ctx context.Context, id int64) (APIKey, error) {
	var k APIKey
	var lastUsed, expires, revoked sql.NullString
	err := s.Store.DB.QueryRowContext(ctx,
		`SELECT id, name, last_used_at, expires_at, revoked_at, created_at FROM client_keys WHERE id=?`, id).
		Scan(&k.ID, &k.Name, &lastUsed, &expires, &revoked, &k.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return APIKey{}, store.ErrNotFound
		}
		return APIKey{}, err
	}
	if lastUsed.Valid {
		v := lastUsed.String
		k.LastUsedAt = &v
	}
	if expires.Valid {
		v := expires.String
		k.ExpiresAt = &v
	}
	if revoked.Valid {
		v := revoked.String
		k.RevokedAt = &v
	}
	return k, nil
}

func (s *Service) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.Store.DB.QueryContext(ctx,
		`SELECT id, name, last_used_at, expires_at, revoked_at, created_at FROM client_keys ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIKey{}
	for rows.Next() {
		var k APIKey
		var lastUsed, expires, revoked sql.NullString
		if err := rows.Scan(&k.ID, &k.Name, &lastUsed, &expires, &revoked, &k.CreatedAt); err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			v := lastUsed.String
			k.LastUsedAt = &v
		}
		if expires.Valid {
			v := expires.String
			k.ExpiresAt = &v
		}
		if revoked.Valid {
			v := revoked.String
			k.RevokedAt = &v
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Service) AuthenticateAPIKey(ctx context.Context, secret string) (APIKey, error) {
	secret = strings.TrimSpace(secret)
	if !strings.HasPrefix(secret, clientKeyPrefix) || len(secret) < len(clientKeyPrefix)+20 {
		return APIKey{}, ErrUnauthorized
	}
	now := store.Now()
	var k APIKey
	var lastUsed, expires, revoked sql.NullString
	err := s.Store.DB.QueryRowContext(ctx, `
		SELECT id, name, last_used_at, expires_at, revoked_at, created_at FROM client_keys
		WHERE key_hash=? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)`,
		crypto.HashToken(secret), now).Scan(
		&k.ID, &k.Name, &lastUsed, &expires, &revoked, &k.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return APIKey{}, ErrUnauthorized
		}
		return APIKey{}, err
	}
	_, _ = s.Store.DB.ExecContext(ctx, `UPDATE client_keys SET last_used_at=? WHERE id=?`, now, k.ID)
	if lastUsed.Valid {
		v := lastUsed.String
		k.LastUsedAt = &v
	}
	return k, nil
}

func (s *Service) RevokeAPIKey(ctx context.Context, id int64) error {
	res, err := s.Store.DB.ExecContext(ctx,
		`UPDATE client_keys SET revoked_at=COALESCE(revoked_at, ?) WHERE id=?`, store.Now(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Service) CreateSession(ctx context.Context, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now().UTC()
	_, _ = s.Store.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now.Format(time.RFC3339Nano))
	_, err := s.Store.DB.ExecContext(ctx,
		`INSERT INTO sessions(token_hash, created_at, expires_at) VALUES(?, ?, ?)`,
		crypto.HashToken(token), now.Format(time.RFC3339Nano), now.Add(ttl).Format(time.RFC3339Nano))
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
	err := s.Store.DB.QueryRowContext(ctx, `SELECT expires_at FROM sessions WHERE token_hash=?`,
		crypto.HashToken(token)).Scan(&expires)
	if err != nil {
		return false
	}
	t, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || !t.After(time.Now().UTC()) {
		_, _ = s.Store.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, crypto.HashToken(token))
		return false
	}
	return true
}

func (s *Service) DeleteSession(ctx context.Context, token string) {
	if token == "" {
		return
	}
	_, _ = s.Store.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, crypto.HashToken(token))
}
