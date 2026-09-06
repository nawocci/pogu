package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/nawocci/pogu/internal/provider"
	"github.com/nawocci/pogu/internal/service"
	"github.com/nawocci/pogu/internal/store"
)

type providerInput struct {
	Name         string               `json:"name"`
	Type         service.ProviderType `json:"type"`
	Prefix       string               `json:"prefix"`
	BaseURL      string               `json:"base_url"`
	KeySelection service.KeySelection `json:"key_selection"`
	APIKey       *string              `json:"api_key"`
	Enabled      *bool                `json:"enabled"`
}

func idParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func keyIDParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("keyId"), 10, 64)
}

func (a *API) providers(w http.ResponseWriter, r *http.Request) {
	out, err := a.Service.ListProviders(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "could not list providers")
		return
	}
	jsonWrite(w, http.StatusOK, out)
}

func (a *API) createProvider(w http.ResponseWriter, r *http.Request) {
	var in providerInput
	if !decodeJSON(w, r, &in) {
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	apiKey := ""
	if in.APIKey != nil {
		apiKey = *in.APIKey
	}
	p, err := a.Service.CreateProvider(r.Context(), in.Name, in.Type, in.Prefix, in.BaseURL, apiKey, enabled, in.KeySelection)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusCreated, p)
}

func (a *API) getProvider(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	p, err := a.Service.GetProvider(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusOK, p)
}

func (a *API) updateProvider(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in providerInput
	if !decodeJSON(w, r, &in) {
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	p, err := a.Service.UpdateProvider(r.Context(), id, in.Name, in.Type, in.Prefix, in.BaseURL, enabled, in.KeySelection)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if p.Builtin != "" && p.Enabled {
		go a.syncBuiltinAsync()
	}
	jsonWrite(w, http.StatusOK, p)
}

func (a *API) deleteProvider(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err = a.Service.DeleteProvider(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) testProvider(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	p, err := a.Service.GetProvider(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	secret, err := a.Service.ProviderPrimarySecret(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			jsonWrite(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "no enabled API keys configured for this provider"})
			return
		}
		jsonError(w, http.StatusInternalServerError, "provider credential unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), provider.HealthTimeout())
	defer cancel()
	if err = a.ProviderClient.Test(ctx, p, secret); err != nil {
		status := http.StatusBadGateway
		if provider.IsAuthFailure(err) {
			status = http.StatusUnauthorized
		}
		jsonWrite(w, status, map[string]any{"ok": false, "message": provider.StatusMessage(err)})
		return
	}
	jsonWrite(w, http.StatusOK, map[string]any{"ok": true, "message": "connection successful"})
}

func (a *API) syncProvider(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	p, err := a.Service.GetProvider(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if p.Builtin == "" {
		jsonError(w, http.StatusBadRequest, "catalog sync is only supported for the built-in provider")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	summary, err := a.Service.SyncOpenCodeModels(ctx)
	if err != nil {
		jsonError(w, http.StatusBadGateway, "catalog sync failed: "+err.Error())
		return
	}
	jsonWrite(w, http.StatusOK, summary)
}

func (a *API) syncBuiltinAsync() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	summary, err := a.Service.SyncOpenCodeModels(ctx)
	if a.Logger == nil {
		return
	}
	if err != nil {
		a.Logger.Warn("opencode catalog sync failed", "error", err.Error())
		return
	}
	a.Logger.Info("opencode catalog sync", "added", summary.Added, "disabled", summary.Disabled, "free", summary.Free)
}
