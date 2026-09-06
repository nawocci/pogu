package httpapi

import (
	"net/http"
)

type modelInput struct {
	ProviderID int64  `json:"provider_id"`
	Name       string `json:"name"`
	Enabled    *bool  `json:"enabled"`
}

func (a *API) listModels(w http.ResponseWriter, r *http.Request) {
	out, err := a.Service.ListModels(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "could not list models")
		return
	}
	jsonWrite(w, http.StatusOK, out)
}

func (a *API) createModel(w http.ResponseWriter, r *http.Request) {
	var in modelInput
	if !decodeJSON(w, r, &in) {
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	m, err := a.Service.CreateModel(r.Context(), in.ProviderID, in.Name, enabled)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusCreated, m)
}

func (a *API) updateModel(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in modelInput
	if !decodeJSON(w, r, &in) {
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	} else {
		current, err := a.Service.GetModel(r.Context(), id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		enabled = current.Enabled
	}
	m, err := a.Service.UpdateModel(r.Context(), id, in.Name, enabled)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusOK, m)
}

func (a *API) deleteModel(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err = a.Service.DeleteModel(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
