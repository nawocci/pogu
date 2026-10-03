package httpapi

import (
	"net/http"

	"github.com/nawocci/pogu/internal/service"
)

func (a *API) listPromptRules(w http.ResponseWriter, r *http.Request) {
	out, err := a.Service.ListPromptRules(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "could not list prompt rules")
		return
	}
	jsonWrite(w, http.StatusOK, out)
}

func (a *API) createPromptRule(w http.ResponseWriter, r *http.Request) {
	var in service.PromptRuleInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := a.Service.CreatePromptRule(r.Context(), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusCreated, out)
}

func (a *API) updatePromptRule(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in service.PromptRuleInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := a.Service.UpdatePromptRule(r.Context(), id, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusOK, out)
}

func (a *API) deletePromptRule(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := a.Service.DeletePromptRule(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) reorderPromptRules(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RuleIDs []int64 `json:"rule_ids"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := a.Service.ReorderPromptRules(r.Context(), in.RuleIDs); err != nil {
		writeServiceError(w, err)
		return
	}
	out, err := a.Service.ListPromptRules(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "could not list prompt rules")
		return
	}
	jsonWrite(w, http.StatusOK, out)
}
