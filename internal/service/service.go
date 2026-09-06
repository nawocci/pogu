package service

import (
	"context"
	"strconv"
	"sync"

	"github.com/nawocci/pogu/internal/crypto"
	"github.com/nawocci/pogu/internal/store"
)

type Service struct {
	Store     *store.Store
	MasterKey []byte

	OpenCodeCatalogURL string
	OpenCodeDocsURL    string

	rrMu      sync.Mutex
	rrCursors map[string]uint64
}

func New(st *store.Store, masterKey []byte) *Service {
	s := &Service{
		Store:     st,
		MasterKey: masterKey,
		rrCursors: make(map[string]uint64),
	}
	s.upgradeLegacyKeyFingerprints(context.Background())
	return s
}

func (s *Service) upgradeLegacyKeyFingerprints(ctx context.Context) {
	if s.Store == nil || s.Store.DB == nil || len(s.MasterKey) == 0 {
		return
	}
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT id, encrypted_secret FROM provider_keys WHERE length(secret_hash) != 32`)
	if err != nil {
		return
	}
	defer rows.Close()
	type item struct {
		id        int64
		encrypted string
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.encrypted); err == nil {
			items = append(items, it)
		}
	}
	for _, it := range items {
		decrypted, err := crypto.Decrypt(s.MasterKey, it.encrypted)
		if err == nil {
			fp := crypto.KeyFingerprint(s.MasterKey, string(decrypted))
			_, _ = s.Store.DB.ExecContext(ctx, `UPDATE provider_keys SET secret_hash=? WHERE id=?`, fp, it.id)
		}
	}
}

func (s *Service) nextRoundRobinCursor(domain string, id int64, n int) int {
	if n <= 1 {
		return 0
	}
	s.rrMu.Lock()
	defer s.rrMu.Unlock()
	if s.rrCursors == nil {
		s.rrCursors = make(map[string]uint64)
	}
	key := domain + ":" + strconv.FormatInt(id, 10)
	curr := s.rrCursors[key]
	s.rrCursors[key] = curr + 1
	return int(curr % uint64(n))
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
