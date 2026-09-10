package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/nawocci/pogu/internal/store"
)

const (
	CavemanSkillURL = "https://raw.githubusercontent.com/JuliusBrussee/caveman/refs/heads/main/skills/caveman/SKILL.md"

	SettingCavemanEnabled  = "caveman_enabled"
	SettingCavemanLevel    = "caveman_level"
	SettingCavemanSkillRaw = "caveman_skill_raw"
	SettingCavemanSyncedAt = "caveman_synced_at"

	DefaultCavemanLevel = "full"
)

var cavemanHTTPClient = &http.Client{Timeout: 15 * time.Second}

var levelRowRegex = regexp.MustCompile(`\|\s*\*\*([a-zA-Z0-9_-]+)\*\*\s*\|\s*([^|]+)\|`)

// CavemanSkill holds parsed sections from the upstream Caveman SKILL.md.
type CavemanSkill struct {
	LeadingInstruction string
	Levels             map[string]string // level ID -> description from Intensity table
	Rules              string
	AutoClarity        string
	Boundaries         string
}

// CavemanLevelMeta holds client-facing metadata for a specific intensity level.
type CavemanLevelMeta struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// CavemanSettings represents router-side Caveman configuration.
type CavemanSettings struct {
	Enabled      bool               `json:"enabled"`
	Level        string             `json:"level"`
	LastSyncedAt string             `json:"last_synced_at"`
	Levels       []CavemanLevelMeta `json:"levels"`
}

// ParseCavemanSkill parses markdown content (like SKILL.md) into a structured CavemanSkill.
func ParseCavemanSkill(raw string) (CavemanSkill, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return CavemanSkill{}, errors.New("empty caveman skill content")
	}

	// Strip YAML frontmatter if present
	if strings.HasPrefix(text, "---") {
		parts := strings.SplitN(text, "---", 3)
		if len(parts) >= 3 {
			text = strings.TrimSpace(parts[2])
		}
	}

	// Break into markdown sections by "## "
	sections := make(map[string]string)
	var currentSection = "_lead"
	var currentLines []string

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			sections[currentSection] = strings.TrimSpace(strings.Join(currentLines, "\n"))
			currentSection = strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			currentLines = nil
		} else {
			currentLines = append(currentLines, line)
		}
	}
	sections[currentSection] = strings.TrimSpace(strings.Join(currentLines, "\n"))

	lead := sections["_lead"]
	levels := make(map[string]string)

	intensityText := sections["Intensity"]
	for _, row := range strings.Split(intensityText, "\n") {
		match := levelRowRegex.FindStringSubmatch(row)
		if len(match) == 3 {
			lvl := strings.ToLower(strings.TrimSpace(match[1]))
			desc := strings.TrimSpace(match[2])
			levels[lvl] = desc
		}
	}

	rules := sections["Rules"]
	autoClarity := sections["Auto-Clarity"]
	boundaries := sections["Boundaries"]

	if len(levels) == 0 {
		return CavemanSkill{}, errors.New("caveman skill missing intensity levels table")
	}
	if rules == "" {
		return CavemanSkill{}, errors.New("caveman skill missing rules section")
	}

	return CavemanSkill{
		LeadingInstruction: lead,
		Levels:             levels,
		Rules:              rules,
		AutoClarity:        autoClarity,
		Boundaries:         boundaries,
	}, nil
}

// LevelMetas returns ordered metadata for all parsed levels suitable for UI/API.
func (s CavemanSkill) LevelMetas() []CavemanLevelMeta {
	preferredOrder := []string{"full", "lite", "ultra", "wenyan-lite", "wenyan-full", "wenyan-ultra"}
	seen := make(map[string]bool)
	var out []CavemanLevelMeta

	for _, id := range preferredOrder {
		if desc, ok := s.Levels[id]; ok {
			seen[id] = true
			out = append(out, CavemanLevelMeta{
				ID:          id,
				Label:       formatLevelLabel(id),
				Description: desc,
			})
		}
	}

	for id, desc := range s.Levels {
		if !seen[id] {
			out = append(out, CavemanLevelMeta{
				ID:          id,
				Label:       formatLevelLabel(id),
				Description: desc,
			})
		}
	}
	return out
}

func formatLevelLabel(id string) string {
	switch id {
	case "full":
		return "Full (Classic Caveman)"
	case "lite":
		return "Lite (Concise Professional)"
	case "ultra":
		return "Ultra (Telegraphic)"
	case "wenyan-lite":
		return "文言 Lite (Classical Chinese)"
	case "wenyan-full":
		return "文言 Full (Classical Chinese)"
	case "wenyan-ultra":
		return "文言 Ultra (Classical Chinese)"
	default:
		return strings.ToUpper(id[:1]) + id[1:]
	}
}

// PromptForLevel synthesizes the precise injected prompt for the chosen intensity level.
func (s CavemanSkill) PromptForLevel(level string) string {
	normalized := strings.ToLower(strings.TrimSpace(level))
	if normalized == "" || normalized == "default" {
		normalized = DefaultCavemanLevel
	}

	desc, ok := s.Levels[normalized]
	if !ok {
		desc = s.Levels[DefaultCavemanLevel]
		if desc == "" {
			desc = "Drop articles, fragments OK, short synonyms. Classic caveman."
		}
	}

	var sb strings.Builder
	lead := s.LeadingInstruction
	if lead == "" {
		lead = "Respond terse like smart caveman. All technical substance stay. Only fluff die."
	}
	sb.WriteString("[Caveman Mode: " + normalized + "]\n")
	sb.WriteString(lead + "\n\n")

	sb.WriteString("Intensity (" + normalized + "):\n")
	sb.WriteString(desc + "\n\n")

	if s.Rules != "" {
		sb.WriteString("Rules:\n")
		sb.WriteString(s.Rules + "\n\n")
	}

	if s.AutoClarity != "" {
		sb.WriteString("Auto-Clarity:\n")
		sb.WriteString(s.AutoClarity + "\n\n")
	}

	if s.Boundaries != "" {
		sb.WriteString("Boundaries:\n")
		sb.WriteString(s.Boundaries + "\n\n")
	}

	sb.WriteString("Reasoning/Thinking:\nIf internal reasoning or thinking is enabled, keep chain-of-thought thorough and deep; apply caveman compression only to the final emitted response.")

	return strings.TrimSpace(sb.String())
}

func (s *Service) cavemanSkillURL() string {
	if s.CavemanSkillURL != "" {
		return s.CavemanSkillURL
	}
	return CavemanSkillURL
}

func (s *Service) ensureCavemanSkill(ctx context.Context) (*CavemanSkill, error) {
	s.cavemanMu.RLock()
	if s.cavemanSkill != nil {
		skill := s.cavemanSkill
		s.cavemanMu.RUnlock()
		return skill, nil
	}
	s.cavemanMu.RUnlock()

	s.cavemanMu.Lock()
	defer s.cavemanMu.Unlock()

	if s.cavemanSkill != nil {
		return s.cavemanSkill, nil
	}

	var raw string
	if s.Store != nil && s.Store.DB != nil {
		err := s.Store.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, SettingCavemanSkillRaw).Scan(&raw)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	if strings.TrimSpace(raw) == "" {
		raw = defaultCavemanSkillMD
	}

	parsed, err := ParseCavemanSkill(raw)
	if err != nil {
		parsed, err = ParseCavemanSkill(defaultCavemanSkillMD)
		if err != nil {
			return nil, fmt.Errorf("parse default caveman skill: %w", err)
		}
	}

	s.cavemanSkill = &parsed
	return s.cavemanSkill, nil
}

// GetCavemanSettings returns current router Caveman configuration.
func (s *Service) GetCavemanSettings(ctx context.Context) (CavemanSettings, error) {
	var enabledFlag, level, syncedAt string
	if s.Store != nil && s.Store.DB != nil {
		_ = s.Store.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, SettingCavemanEnabled).Scan(&enabledFlag)
		_ = s.Store.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, SettingCavemanLevel).Scan(&level)
		_ = s.Store.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, SettingCavemanSyncedAt).Scan(&syncedAt)
	}

	if level == "" {
		level = DefaultCavemanLevel
	}

	skill, err := s.ensureCavemanSkill(ctx)
	if err != nil {
		return CavemanSettings{}, err
	}

	return CavemanSettings{
		Enabled:      enabledFlag == "1",
		Level:        level,
		LastSyncedAt: syncedAt,
		Levels:       skill.LevelMetas(),
	}, nil
}

// SetCavemanSettings updates the router-wide Caveman enabled flag and default level.
func (s *Service) SetCavemanSettings(ctx context.Context, enabled bool, level string) error {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		level = DefaultCavemanLevel
	}

	skill, err := s.ensureCavemanSkill(ctx)
	if err != nil {
		return err
	}
	if _, ok := skill.Levels[level]; !ok && level != DefaultCavemanLevel {
		return fmt.Errorf("%w: unknown caveman level %q", ErrValidation, level)
	}

	now := store.Now()
	flag := "0"
	if enabled {
		flag = "1"
	}

	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for k, v := range map[string]string{
		SettingCavemanEnabled: flag,
		SettingCavemanLevel:   level,
	} {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO settings(key, value, updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
			k, v, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SyncCavemanSkill fetches the latest upstream SKILL.md from GitHub, parses and validates it,
// and saves it into the settings table.
func (s *Service) SyncCavemanSkill(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cavemanSkillURL(), nil)
	if err != nil {
		return err
	}
	resp, err := cavemanHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch caveman skill: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fetch caveman skill: status %d", resp.StatusCode)
	}

	const maxBytes = 1 << 20 // 1MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return fmt.Errorf("read caveman skill: %w", err)
	}

	raw := string(body)
	parsed, err := ParseCavemanSkill(raw)
	if err != nil {
		return fmt.Errorf("parse caveman skill: %w", err)
	}

	now := store.Now()
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for k, v := range map[string]string{
		SettingCavemanSkillRaw: raw,
		SettingCavemanSyncedAt: now,
	} {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO settings(key, value, updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
			k, v, now); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	s.cavemanMu.Lock()
	s.cavemanSkill = &parsed
	s.cavemanMu.Unlock()

	return nil
}

// GetCavemanPrompt returns the synthesized prompt for a given intensity level.
func (s *Service) GetCavemanPrompt(ctx context.Context, level string) (string, error) {
	skill, err := s.ensureCavemanSkill(ctx)
	if err != nil {
		return "", err
	}
	return skill.PromptForLevel(level), nil
}

// ActiveCavemanPrompt returns the prompt for the configured level if router-wide Caveman is enabled.
func (s *Service) ActiveCavemanPrompt(ctx context.Context) (string, error) {
	var enabledFlag, level string
	if s.Store != nil && s.Store.DB != nil {
		_ = s.Store.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, SettingCavemanEnabled).Scan(&enabledFlag)
		_ = s.Store.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, SettingCavemanLevel).Scan(&level)
	}
	if enabledFlag != "1" {
		return "", nil
	}
	if level == "" {
		level = DefaultCavemanLevel
	}
	return s.GetCavemanPrompt(ctx, level)
}
