package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/nawocci/pogu/internal/store"
)

const (
	BuiltinOpenCodeFree   = "opencode-free"
	BuiltinOpenCodePrefix = "oc"
	BuiltinOpenCodeName   = "OpenCode Free"
	BuiltinOpenCodeBase   = "https://opencode.ai/zen"
	BuiltinOpenCodeKey    = "public"
)

func (s *Service) EnsureBuiltin(ctx context.Context) error {
	var id int64
	err := s.Store.DB.QueryRowContext(ctx, `SELECT id FROM providers WHERE builtin=?`, BuiltinOpenCodeFree).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return s.createBuiltin(ctx)
	}
	if err != nil {
		return err
	}
	var enabled int
	err = s.Store.DB.QueryRowContext(ctx, `SELECT 1 FROM provider_keys WHERE provider_id=? AND enabled=1 LIMIT 1`, id).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = s.CreateProviderKey(ctx, id, "Public", BuiltinOpenCodeKey)
		return err
	}
	return err
}

func (s *Service) createBuiltin(ctx context.Context) error {
	now := store.Now()
	result, err := s.Store.DB.ExecContext(ctx,
		`INSERT INTO providers(name,type,prefix,base_url,key_selection,enabled,created_at,updated_at,builtin) VALUES(?,?,?,?,?,?,?,?,?)`,
		BuiltinOpenCodeName, ProviderOpenAI, BuiltinOpenCodePrefix, BuiltinOpenCodeBase,
		KeySelectionFirst, 0, now, now, BuiltinOpenCodeFree)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	_, err = s.CreateProviderKey(ctx, id, "Public", BuiltinOpenCodeKey)
	return err
}
