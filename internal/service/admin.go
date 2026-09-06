package service

import (
	"context"
	"database/sql"
	"errors"
	"unicode/utf8"

	"github.com/nawocci/pogu/internal/crypto"
	"github.com/nawocci/pogu/internal/store"
)

func (s *Service) SetAdminPassword(ctx context.Context, password string) error {
	if utf8.RuneCountInString(password) < 12 {
		return validation("password must be at least 12 characters")
	}
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return err
	}
	now := store.Now()
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO admin(id, password_hash, created_at, updated_at) VALUES(1, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET password_hash=excluded.password_hash, updated_at=excluded.updated_at`,
		hash, now, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) InitializeAdmin(ctx context.Context, password string) error {
	if utf8.RuneCountInString(password) < 12 {
		return validation("password must be at least 12 characters")
	}
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return err
	}
	now := store.Now()
	_, err = s.Store.DB.ExecContext(ctx,
		`INSERT INTO admin(id, password_hash, created_at, updated_at) VALUES(1, ?, ?, ?)`, hash, now, now)
	if err != nil {
		if isUnique(err) {
			return &ValidationError{"admin already exists"}
		}
		return err
	}
	return nil
}

func (s *Service) CheckAdminPassword(ctx context.Context, password string) (bool, error) {
	var hash string
	err := s.Store.DB.QueryRowContext(ctx, `SELECT password_hash FROM admin WHERE id=1`).Scan(&hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return crypto.CheckPassword(password, hash), nil
}
