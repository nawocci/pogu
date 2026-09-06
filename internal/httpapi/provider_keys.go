package httpapi

import (
	"context"
	"net/http"

	"github.com/nawocci/pogu/internal/provider"
)

type providerKeyCreateInput struct {
	Name   string `json:"name"`
	Secret string `json:"secret"`
}

type providerKeyUpdateInput struct {
	Name    string `json:"name"`
	Enabled *bool  `json:"enabled"`
}

func (a *API) listProviderKeys(w http.ResponseWriter, r *http.Request) {
	providerID, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	keys, err := a.Service.ListProviderKeys(r.Context(), providerID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusOK, keys)
}

func (a *API) createProviderKey(w http.ResponseWriter, r *http.Request) {
	providerID, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in providerKeyCreateInput
	if !decodeJSON(w, r, &in) {
		return
	}
	key, err := a.Service.CreateProviderKey(r.Context(), providerID, in.Name, in.Secret)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusCreated, map[string]any{"key": key, "secret": in.Secret})
}

func (a *API) updateProviderKey(w http.ResponseWriter, r *http.Request) {
	keyID, err := keyIDParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid key id")
		return
	}
	var in providerKeyUpdateInput
	if !decodeJSON(w, r, &in) {
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	key, err := a.Service.UpdateProviderKey(r.Context(), keyID, in.Name, enabled)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusOK, key)
}

func (a *API) deleteProviderKey(w http.ResponseWriter, r *http.Request) {
	keyID, err := keyIDParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid key id")
		return
	}
	if err := a.Service.DeleteProviderKey(r.Context(), keyID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) makeProviderKeyPrimary(w http.ResponseWriter, r *http.Request) {
	providerID, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	keyID, err := keyIDParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid key id")
		return
	}
	if err := a.Service.SetProviderKeyPrimary(r.Context(), providerID, keyID); err != nil {
		writeServiceError(w, err)
		return
	}
	keys, err := a.Service.ListProviderKeys(r.Context(), providerID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusOK, keys)
}

func (a *API) testProviderKey(w http.ResponseWriter, r *http.Request) {
	providerID, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	keyID, err := keyIDParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid key id")
		return
	}
	p, err := a.Service.GetProvider(r.Context(), providerID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	secret, err := a.Service.ProviderKeySecret(r.Context(), keyID)
	if err != nil {
		writeServiceError(w, err)
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
