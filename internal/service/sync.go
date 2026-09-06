package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/nawocci/pogu/internal/store"
)

// OpenCodeCatalogURL serves the full upstream model catalog in OpenAI list
// shape ({object:"list",data:[{id,...}]}).
const OpenCodeCatalogURL = "https://opencode.ai/zen/v1/models"

// OpenCodeSyncSummary describes one catalog reconciliation.
type OpenCodeSyncSummary struct {
	Catalog  int `json:"catalog"`
	Free     int `json:"free"`
	Added    int `json:"added"`
	Disabled int `json:"disabled"`
}

var openCodeHTTPClient = &http.Client{Timeout: 20 * time.Second}

func (s *Service) openCodeCatalogURL() string {
	if s.OpenCodeCatalogURL != "" {
		return s.OpenCodeCatalogURL
	}
	return OpenCodeCatalogURL
}

func (s *Service) GetBuiltinProvider(ctx context.Context) (Provider, error) {
	if err := s.EnsureBuiltin(ctx); err != nil {
		return Provider{}, err
	}
	var id int64
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT id FROM providers WHERE builtin=?`, BuiltinOpenCodeFree).Scan(&id); err != nil {
		return Provider{}, err
	}
	return s.GetProvider(ctx, id)
}

// SyncOpenCodeModels reconciles the built-in provider's model rows with the
// live upstream catalog intersected with the Zen docs free list: docs-Free
// ids present in the catalog are added (enabled) with the wire scheme seeded
// from the docs endpoint URL. Locally stored ids missing from that set are
// disabled (never deleted, so group memberships and telemetry history
// survive). Existing rows keep their enabled state and explicit scheme pins —
// an operator disable or pin is not overridden. Both sources are required: a
// fetch or parse failure aborts the sync and changes nothing. The caller
// decides when syncing is appropriate (enabled-gating lives in the worker,
// not here, so a manual sync is always an explicit opt-in).
func (s *Service) SyncOpenCodeModels(ctx context.Context) (OpenCodeSyncSummary, error) {
	var summary OpenCodeSyncSummary
	p, err := s.GetBuiltinProvider(ctx)
	if err != nil {
		return summary, err
	}
	ids, err := fetchOpenCodeCatalog(ctx, s.openCodeCatalogURL())
	if err != nil {
		return summary, err
	}
	summary.Catalog = len(ids)
	docs, err := fetchOpenCodeDocs(ctx, s.openCodeDocsURL())
	if err != nil {
		return summary, err
	}
	free := docs.FreeIDs()
	want := make(map[string]string)
	for _, id := range ids {
		if !free[id] {
			continue
		}
		want[id] = SchemeForDocsEndpoint(docs.Endpoints[id])
	}
	summary.Free = len(want)

	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return summary, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT name, scheme FROM models WHERE provider_id=?`, p.ID)
	if err != nil {
		return summary, err
	}
	have := make(map[string]bool)
	schemes := make(map[string]string)
	for rows.Next() {
		var name, scheme string
		if err := rows.Scan(&name, &scheme); err != nil {
			_ = rows.Close()
			return summary, err
		}
		have[name] = true
		schemes[name] = scheme
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return summary, err
	}
	now := store.Now()
	for name, scheme := range want {
		if have[name] {
			if schemes[name] == "" && scheme != "" {
				if _, err := tx.ExecContext(ctx, `UPDATE models SET scheme=?,updated_at=? WHERE provider_id=? AND name=?`, scheme, now, p.ID, name); err != nil {
					return summary, err
				}
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO models(provider_id,name,enabled,created_at,updated_at,scheme) VALUES(?,?,1,?,?,?)`, p.ID, name, now, now, scheme); err != nil {
			return summary, err
		}
		summary.Added++
	}
	for name := range have {
		if _, ok := want[name]; ok {
			continue
		}
		res, err := tx.ExecContext(ctx, `UPDATE models SET enabled=0,updated_at=? WHERE provider_id=? AND name=? AND enabled=1`, now, p.ID, name)
		if err != nil {
			return summary, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			summary.Disabled++
		}
	}
	if err := tx.Commit(); err != nil {
		return summary, err
	}
	return summary, nil
}

func fetchOpenCodeCatalog(ctx context.Context, catalogURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, catalogURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := openCodeHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch OpenCode catalog: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch OpenCode catalog: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read OpenCode catalog: %w", err)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode OpenCode catalog: %w", err)
	}
	if payload.Data == nil {
		return nil, errors.New("decode OpenCode catalog: missing data field")
	}
	ids := make([]string, 0, len(payload.Data))
	for _, m := range payload.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}
