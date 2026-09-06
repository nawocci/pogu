package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/nawocci/pogu/internal/crypto"
	"github.com/nawocci/pogu/internal/store"
)

func (s *Service) SetAdminPassword(ctx context.Context, password string) error {
	if utf8.RuneCountInString(password) < 12 {
		return fmt.Errorf("%w: administrator password must be at least 12 characters", ErrValidation)
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
	if _, err := tx.ExecContext(ctx, `INSERT INTO admin(id,password_hash,created_at,updated_at) VALUES(1,?,?,?) ON CONFLICT(id) DO UPDATE SET password_hash=excluded.password_hash,updated_at=excluded.updated_at`, hash, now, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) InitializeAdmin(ctx context.Context, password string) error {
	if utf8.RuneCountInString(password) < 12 {
		return fmt.Errorf("%w: administrator password must be at least 12 characters", ErrValidation)
	}
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO admin(id,password_hash,created_at,updated_at) VALUES(1,?,?,?)`, hash, store.Now(), store.Now())
	if err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "unique") || strings.Contains(lower, "constraint") {
			return fmt.Errorf("%w: administrator already initialized", ErrAlreadyExists)
		}
		return err
	}
	return tx.Commit()
}

func (s *Service) AdminInitialized(ctx context.Context) (bool, error) {
	var present bool
	err := s.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin WHERE id=1)`).Scan(&present)
	if err != nil {
		return false, err
	}
	return present, nil
}

func (s *Service) CheckAdminPassword(ctx context.Context, password string) (bool, error) {
	var hash string
	err := s.Store.DB.QueryRowContext(ctx, `SELECT password_hash FROM admin WHERE id=1`).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return crypto.CheckPassword(password, hash), nil
}
