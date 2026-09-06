package httpapi

import (
	"errors"
	"net/http"

	"github.com/nawocci/pogu/internal/service"
)

func (a *API) getGlobalPrompt(w http.ResponseWriter, r *http.Request) {
	text, enabled, err := a.Service.GetGlobalPrompt(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "could not load settings")
		return
	}
	jsonWrite(w, http.StatusOK, map[string]any{"system_prompt": text, "enabled": enabled})
}

func (a *API) updateGlobalPrompt(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SystemPrompt string `json:"system_prompt"`
		Enabled      bool   `json:"enabled"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := a.Service.SetGlobalPrompt(r.Context(), in.SystemPrompt, in.Enabled); err != nil {
		writeServiceError(w, err)
		return
	}
	text, enabled, err := a.Service.GetGlobalPrompt(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "could not load settings")
		return
	}
	jsonWrite(w, http.StatusOK, map[string]any{"system_prompt": text, "enabled": enabled})
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := a.Service.ChangeAdminPassword(r.Context(), in.CurrentPassword, in.NewPassword); err != nil {
		if errors.Is(err, service.ErrUnauthorized) {
			jsonError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusOK, map[string]any{"changed": true})
}
