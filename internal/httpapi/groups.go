package httpapi

import (
	"net/http"
	"strconv"

	"github.com/nawocci/pogu/internal/service"
)

type groupInput struct {
	Name      string               `json:"name"`
	Selection service.KeySelection `json:"selection"`
	Enabled   *bool                `json:"enabled"`
}

func (in groupInput) enabledOr(def bool) bool {
	if in.Enabled == nil {
		return def
	}
	return *in.Enabled
}

func (a *API) listGroups(w http.ResponseWriter, r *http.Request) {
	out, err := a.Service.ListGroups(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "could not list groups")
		return
	}
	jsonWrite(w, http.StatusOK, out)
}

func (a *API) createGroup(w http.ResponseWriter, r *http.Request) {
	var in groupInput
	if !decodeJSON(w, r, &in) {
		return
	}
	g, err := a.Service.CreateGroup(r.Context(), in.Name, in.enabledOr(true), in.Selection)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusCreated, g)
}

func (a *API) getGroup(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	g, err := a.Service.GetGroup(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusOK, g)
}

func (a *API) updateGroup(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in groupInput
	if !decodeJSON(w, r, &in) {
		return
	}
	g, err := a.Service.UpdateGroup(r.Context(), id, in.Name, in.enabledOr(true), in.Selection)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusOK, g)
}

func (a *API) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err = a.Service.DeleteGroup(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listGroupMembers(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	out, err := a.Service.ListGroupMembers(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusOK, out)
}

func (a *API) addGroupMember(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in struct {
		ModelID int64 `json:"model_id"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	m, err := a.Service.AddGroupMember(r.Context(), id, in.ModelID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusCreated, m)
}

func memberIDParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("memberId"), 10, 64)
}

func (a *API) updateGroupMember(w http.ResponseWriter, r *http.Request) {
	id, memberID, ok := groupMemberParams(w, r)
	if !ok {
		return
	}
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	m, err := a.Service.UpdateGroupMember(r.Context(), id, memberID, in.Enabled == nil || *in.Enabled)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusOK, m)
}

func (a *API) deleteGroupMember(w http.ResponseWriter, r *http.Request) {
	id, memberID, ok := groupMemberParams(w, r)
	if !ok {
		return
	}
	if err := a.Service.DeleteGroupMember(r.Context(), id, memberID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) reorderGroupMembers(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in struct {
		MemberIDs []int64 `json:"member_ids"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := a.Service.ReorderGroupMembers(r.Context(), id, in.MemberIDs); err != nil {
		writeServiceError(w, err)
		return
	}
	out, err := a.Service.ListGroupMembers(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusOK, out)
}

func (a *API) setGroupSelection(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in struct {
		Selection service.KeySelection `json:"selection"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	g, err := a.Service.SetGroupSelection(r.Context(), id, in.Selection)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	jsonWrite(w, http.StatusOK, g)
}

func groupMemberParams(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
	id, err := idParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return 0, 0, false
	}
	memberID, err := memberIDParam(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid member id")
		return 0, 0, false
	}
	return id, memberID, true
}
