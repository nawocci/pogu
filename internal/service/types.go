package service

type ProviderType string

const (
	ProviderOpenAI          ProviderType = "openai"
	ProviderOpenAIResponses ProviderType = "openai-responses"
	ProviderAnthropic       ProviderType = "anthropic"
)

func (t ProviderType) Valid() bool {
	switch t {
	case ProviderOpenAI, ProviderAnthropic:
		return true
	default:
		return false
	}
}

type KeySelection string

const (
	SelectionFirst KeySelection = "first"
	SelectionRound KeySelection = "round_robin"
)

func (s KeySelection) Valid() bool {
	return s == SelectionFirst || s == SelectionRound
}

type Scheme string

const (
	SchemeOpenAI          Scheme = "openai"
	SchemeOpenAIResponses Scheme = "openai-responses"
	SchemeAnthropic       Scheme = "anthropic"
)

func (s Scheme) Valid() bool {
	switch s {
	case "", SchemeOpenAI, SchemeAnthropic:
		return true
	default:
		return false
	}
}

func DefaultScheme(t ProviderType) Scheme {
	if t == ProviderAnthropic {
		return SchemeAnthropic
	}
	return SchemeOpenAI
}

func EffectiveScheme(providerType ProviderType, modelScheme string) Scheme {
	if Scheme(modelScheme).Valid() && modelScheme != "" {
		return Scheme(modelScheme)
	}
	return DefaultScheme(providerType)
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
	CreatedAt    string       `json:"created_at"`
	UpdatedAt    string       `json:"updated_at"`
}

type ProviderKey struct {
	ID         int64   `json:"id"`
	ProviderID int64   `json:"provider_id"`
	Name       string  `json:"name"`
	MaskedKey  string  `json:"masked_key"`
	Enabled    bool    `json:"enabled"`
	SortOrder  int     `json:"sort_order"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

type Model struct {
	ID         int64  `json:"id"`
	ProviderID int64  `json:"provider_id"`
	Provider   string `json:"provider"`
	Prefix     string `json:"prefix"`
	Name       string `json:"name"`
	PublicID   string `json:"public_id"`
	Enabled    bool   `json:"enabled"`
	Scheme     string `json:"scheme"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type APIKey struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	LastUsedAt *string `json:"last_used_at"`
	ExpiresAt  *string `json:"expires_at"`
	RevokedAt  *string `json:"revoked_at"`
	CreatedAt  string  `json:"created_at"`
}

type Route struct {
	Provider Provider
	Model    Model
}
