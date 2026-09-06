package control

import (
	"context"
	"encoding/json"

	"github.com/nawocci/pogu/internal/service"
)

type Status struct {
	Providers int `json:"providers"`
	Models    int `json:"models"`
	Keys      int `json:"keys"`
}

const TelemetryLimit = 50

type ProviderTestResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type KeyCreateResult struct {
	Key    service.APIKey `json:"key"`
	Secret string         `json:"secret"`
}

type Request struct {
	Op string `json:"op"`
	ID int64  `json:"id,omitempty"`

	Name         string               `json:"name,omitempty"`
	Type         service.ProviderType `json:"type,omitempty"`
	Prefix       string               `json:"prefix,omitempty"`
	BaseURL      string               `json:"base_url,omitempty"`
	KeySelection service.KeySelection `json:"key_selection,omitempty"`
	APIKey       *string              `json:"api_key,omitempty"`
	Secret       *string              `json:"secret,omitempty"`
	Text         string               `json:"text,omitempty"`
	Preview      bool                 `json:"preview,omitempty"`
	Enabled      *bool                `json:"enabled,omitempty"`

	ProviderID int64 `json:"provider_id,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Response struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

type ProviderTestFunc func(context.Context, service.Provider, string) error

type ProviderTester interface {
	Test(context.Context, service.Provider, string) error
}
