package service

import "time"

type ProviderType string

const (
	ProviderOpenAI          ProviderType = "openai"
	ProviderOpenAIResponses ProviderType = "openai-responses"
	ProviderAnthropic       ProviderType = "anthropic"
)

func (t ProviderType) Valid() bool {
	return t == ProviderOpenAI || t == ProviderOpenAIResponses || t == ProviderAnthropic
}

type KeySelection string

const (
	KeySelectionFirst      KeySelection = "first"
	KeySelectionRoundRobin KeySelection = "round_robin"
)

func (s KeySelection) Valid() bool {
	return s == KeySelectionFirst || s == KeySelectionRoundRobin
}

type Scheme string

const (
	SchemeOpenAI          Scheme = "openai"
	SchemeOpenAIResponses Scheme = "openai-responses"
	SchemeAnthropic       Scheme = "anthropic"
)

func (s Scheme) Valid() bool {
	switch s {
	case "", SchemeOpenAI, SchemeOpenAIResponses, SchemeAnthropic:
		return true
	default:
		return false
	}
}

func ValidModelScheme(scheme string) bool {
	return Scheme(scheme).Valid()
}

func DefaultScheme(t ProviderType) Scheme {
	if t == ProviderAnthropic {
		return SchemeAnthropic
	}
	return SchemeOpenAI
}

func EffectiveScheme(p Provider, m Model) Scheme {
	if Scheme(m.Scheme).Valid() && m.Scheme != "" {
		return Scheme(m.Scheme)
	}
	return DefaultScheme(p.Type)
}

type Provider struct {
	ID           int64        `json:"id"`
	Name         string       `json:"name"`
	Type         ProviderType `json:"type"`
	Prefix       string       `json:"prefix"`
	BaseURL      string       `json:"base_url"`
	HasAPIKey    bool         `json:"has_api_key"`
	KeyCount     int          `json:"key_count"`
	KeySelection KeySelection `json:"key_selection"`
	Enabled      bool         `json:"enabled"`
	Builtin      string       `json:"builtin"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

type ProviderKey struct {
	ID         int64      `json:"id"`
	ProviderID int64      `json:"provider_id"`
	Name       string     `json:"name"`
	MaskedKey  string     `json:"masked_key"`
	Enabled    bool       `json:"enabled"`
	SortOrder  int        `json:"sort_order"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type Model struct {
	ID         int64     `json:"id"`
	ProviderID int64     `json:"provider_id"`
	Provider   string    `json:"provider"`
	Prefix     string    `json:"prefix"`
	Name       string    `json:"name"`
	PublicID   string    `json:"public_id"`
	Enabled    bool      `json:"enabled"`
	Scheme     string    `json:"scheme"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type APIKey struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type Group struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	Selection   KeySelection `json:"selection"`
	Enabled     bool         `json:"enabled"`
	MemberCount int          `json:"member_count"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type GroupMember struct {
	ID        int64     `json:"id"`
	GroupID   int64     `json:"group_id"`
	ModelID   int64     `json:"model_id"`
	Position  int       `json:"position"`
	Enabled   bool      `json:"enabled"`
	PublicID  string    `json:"public_id"`
	Provider  string    `json:"provider"`
	Prefix    string    `json:"prefix"`
	ModelName string    `json:"model_name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ImportEntry struct {
	LineNumber int    `json:"line_number"`
	Name       string `json:"name"`
	MaskedKey  string `json:"masked_key"`
	Valid      bool   `json:"valid"`
	Error      string `json:"error,omitempty"`
}

type ImportResult struct {
	Total   int           `json:"total"`
	Valid   int           `json:"valid"`
	Invalid int           `json:"invalid"`
	Entries []ImportEntry `json:"entries"`
}

type Route struct {
	Provider Provider
	Model    Model
}
