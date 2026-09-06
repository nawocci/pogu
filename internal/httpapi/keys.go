package httpapi

import (
	"net/http"
)

func (a *API) listKeys(w http.ResponseWriter, r *http.Request) {
	out, err := a.Service.ListAPIKeys(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "could not list keys")
		return
	}
	jsonWrite(w, http.StatusOK, out)
}

func (a *API) createKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	key, secret, err := a.Service.CreateAPIKey(r.Context(), in.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusCreated, map[string]any{"key": key, "secret": secret})
}

func (a *API) revokeKey(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err = a.Service.RevokeAPIKey(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusOK, map[string]any{"revoked": true})
}
