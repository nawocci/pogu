package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/nawocci/pogu/internal/crypto"
	"github.com/nawocci/pogu/internal/store"
)

type validParsedEntry struct {
	name      string
	secret    string
	fp        []byte
	maskedKey string
}

func (s *Service) ImportProviderKeys(ctx context.Context, providerID int64, text string, commit bool) (ImportResult, error) {
	var exists int
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT 1 FROM providers WHERE id=?`, providerID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return ImportResult{}, store.ErrNotFound
	} else if err != nil {
		return ImportResult{}, err
	}

	rows, err := s.Store.DB.QueryContext(ctx, `SELECT name, secret_hash FROM provider_keys WHERE provider_id=?`, providerID)
	if err != nil {
		return ImportResult{}, err
	}
	defer rows.Close()
	existingFPs := make(map[string]struct{})
	maxN := 0
	for rows.Next() {
		var name string
		var fp []byte
		if err := rows.Scan(&name, &fp); err == nil {
			existingFPs[string(fp)] = struct{}{}
			if m := keyNamePattern.FindStringSubmatch(name); len(m) == 2 {
				if n, err := strconv.Atoi(m[1]); err == nil && n > maxN {
					maxN = n
				}
			}
		}
	}

	lines := strings.Split(text, "\n")
	var entries []ImportEntry
	var validEntries []validParsedEntry
	batchFPs := make(map[string]int)

	nextAutoNum := maxN

	for idx, rawLine := range lines {
		lineNum := idx + 1
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		name := ""
		secret := ""
		if strings.Contains(line, "|") {
			parts := strings.SplitN(line, "|", 2)
			name = strings.TrimSpace(parts[0])
			secret = strings.TrimSpace(parts[1])
		} else {
			secret = line
		}

		entry := ImportEntry{
			LineNumber: lineNum,
			Name:       name,
			MaskedKey:  crypto.MaskSecret(secret),
		}

		if secret == "" {
			entry.Valid = false
			entry.Error = "empty secret"
			entries = append(entries, entry)
			continue
		}

		fp := crypto.KeyFingerprint(s.MasterKey, secret)
		fpKey := string(fp)

		if _, exists := existingFPs[fpKey]; exists {
			entry.Valid = false
			entry.Error = "duplicate secret already exists for provider"
			entries = append(entries, entry)
			continue
		}

		if firstLine, dup := batchFPs[fpKey]; dup {
			entry.Valid = false
			entry.Error = fmt.Sprintf("duplicate secret in batch (line %d)", firstLine)
			entries = append(entries, entry)
			continue
		}

		batchFPs[fpKey] = lineNum

		if name == "" {
			nextAutoNum++
			name = fmt.Sprintf("Key %d", nextAutoNum)
			entry.Name = name
		}

		entry.Valid = true
		entries = append(entries, entry)
		validEntries = append(validEntries, validParsedEntry{
			name:      name,
			secret:    secret,
			fp:        fp,
			maskedKey: entry.MaskedKey,
		})
	}

	validCount := len(validEntries)
	invalidCount := len(entries) - validCount
	result := ImportResult{
		Total:   len(entries),
		Valid:   validCount,
		Invalid: invalidCount,
		Entries: entries,
	}

	if commit && validCount > 0 {
		var maxSort int
		_ = s.Store.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), -1) FROM provider_keys WHERE provider_id=?`, providerID).Scan(&maxSort)
		tx, err := s.Store.DB.BeginTx(ctx, nil)
		if err != nil {
			return ImportResult{}, err
		}
		defer tx.Rollback()
		now := store.Now()
		for _, v := range validEntries {
			encSecret, err := crypto.Encrypt(s.MasterKey, []byte(v.secret))
			if err != nil {
				return ImportResult{}, fmt.Errorf("encrypt secret: %w", err)
			}
			maxSort++
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO provider_keys(provider_id, name, encrypted_secret, secret_hash, enabled, sort_order, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?)`,
				providerID, v.name, encSecret, v.fp, 1, maxSort, now, now); err != nil {
				return ImportResult{}, err
			}
		}
		if err := tx.Commit(); err != nil {
			return ImportResult{}, err
		}
	}

	return result, nil
}
