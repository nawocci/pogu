package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/nawocci/pogu/internal/store"
)

const (
	SettingGlobalPrompt        = "global_system_prompt"
	SettingGlobalPromptEnabled = "global_system_prompt_enabled"
	maxSystemPromptRunes       = 16384
)

func (s *Service) GetGlobalPrompt(ctx context.Context) (text string, enabled bool, err error) {
	var raw, flag string
	err = s.Store.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, SettingGlobalPrompt).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	err = s.Store.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, SettingGlobalPromptEnabled).Scan(&flag)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	return raw, flag == "1", nil
}

func (s *Service) SetGlobalPrompt(ctx context.Context, text string, enabled bool) error {
	if utf8.RuneCountInString(text) > maxSystemPromptRunes {
		return fmt.Errorf("%w: system prompt must be at most 16384 characters", ErrValidation)
	}
	now := store.Now()
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	flag := "0"
	if enabled {
		flag = "1"
	}
	for key, value := range map[string]string{SettingGlobalPrompt: text, SettingGlobalPromptEnabled: flag} {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO settings(key, value, updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
			key, value, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func ActivePrompt(text string, enabled bool) string {
	if !enabled || strings.TrimSpace(text) == "" {
		return ""
	}
	return text
}

func (s *Service) ChangeAdminPassword(ctx context.Context, current, next string) error {
	ok, err := s.CheckAdminPassword(ctx, current)
	if err != nil {
		return err
	}
	if !ok {
		return ErrUnauthorized
	}
	return s.SetAdminPassword(ctx, next)
}
