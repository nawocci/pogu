package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const openCodeSyncInterval = 7 * 24 * time.Hour

const OpenCodeDocsCacheFile = "opencode-docs-cache.json"

type docsCacheFile struct {
	FetchedAt time.Time         `json:"fetched_at"`
	Endpoints map[string]string `json:"endpoints"`
}

func (s *Service) docsCachePath() string {
	if s.OpenCodeDocsCacheFile != "" {
		return s.OpenCodeDocsCacheFile
	}
	return ""
}

func (s *Service) loadDocsCache() (DocsModels, time.Time, error) {
	var out DocsModels
	var zero time.Time
	path := s.docsCachePath()
	if path == "" {
		return out, zero, errors.New("docs cache is not configured")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out, zero, err
	}
	var cached docsCacheFile
	if err := json.Unmarshal(raw, &cached); err != nil {
		return out, zero, err
	}
	if cached.Endpoints == nil {
		return out, zero, errors.New("docs cache has no endpoints")
	}
	out.Endpoints = cached.Endpoints
	return out, cached.FetchedAt, nil
}

func (s *Service) saveDocsCache(docs DocsModels) error {
	path := s.docsCachePath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(docsCacheFile{FetchedAt: time.Now().UTC(), Endpoints: docs.Endpoints}, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// AutoSyncOpenCodeModels skips the docs fetch while the cache is fresh;
// any failure leaves the database untouched.
func (s *Service) AutoSyncOpenCodeModels(ctx context.Context) (OpenCodeSyncSummary, error) {
	var summary OpenCodeSyncSummary
	if cached, at, err := s.loadDocsCache(); err == nil && time.Since(at) < openCodeSyncInterval {
		p, err := s.GetBuiltinProvider(ctx)
		if err != nil {
			return summary, err
		}
		ids, err := fetchOpenCodeCatalog(ctx, s.openCodeCatalogURL())
		if err != nil {
			return summary, err
		}
		summary.Catalog = len(ids)
		free := cached.FreeIDs()
		want := make(map[string]string)
		for _, id := range ids {
			if !free[id] {
				continue
			}
			want[id] = SchemeForDocsEndpoint(cached.Endpoints[id])
		}
		summary.Free = len(want)
		return s.reconcileCatalog(ctx, p, want, summary)
	}
	return s.SyncOpenCodeModels(ctx)
}
