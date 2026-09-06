package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/nawocci/pogu/internal/service"
	"github.com/nawocci/pogu/internal/store"
)

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if r.Body == nil {
		jsonError(w, http.StatusBadRequest, "request body required")
		return false
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON request")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		jsonError(w, http.StatusBadRequest, "request must contain exactly one JSON value")
		return false
	}
	return true
}

func jsonWrite(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func jsonError(w http.ResponseWriter, status int, message string) {
	jsonWrite(w, status, map[string]any{"error": map[string]string{"message": message, "type": "pogu_error"}})
}

func writeProtocolError(w http.ResponseWriter, protocol string, status int, message, typ string) {
	if protocol == "anthropic" {
		jsonWrite(w, status, map[string]any{"type": "error", "error": map[string]string{"type": typ, "message": message}})
		return
	}
	jsonWrite(w, status, map[string]any{"error": map[string]string{"message": message, "type": typ}})
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrValidation), errors.Is(err, service.ErrAlreadyExists), errors.Is(err, service.ErrBuiltin):
		jsonError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrNotFound):
		jsonError(w, http.StatusNotFound, "not found")
	default:
		jsonError(w, http.StatusInternalServerError, "internal server error")
	}
}

func upstreamErrorType(protocol string) string {
	if protocol == "anthropic" {
		return "api_error"
	}
	return "upstream_error"
}

func internalErrorType(protocol string) string {
	if protocol == "anthropic" {
		return "api_error"
	}
	return "internal_error"
}
