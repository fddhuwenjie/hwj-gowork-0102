package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"weld-ndt/internal/domain"
	"weld-ndt/internal/platform"
	"weld-ndt/internal/store"
)

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, code, message string, details any) {
	WriteJSON(w, status, map[string]any{"error": platform.NewAPIError(code, message, details)})
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidTransition):
		WriteError(w, http.StatusConflict, "invalid_transition", err.Error(), nil)
	case errors.Is(err, store.ErrVersionConflict):
		WriteError(w, http.StatusConflict, "version_conflict", err.Error(), nil)
	case errors.Is(err, store.ErrNotFound):
		WriteError(w, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, domain.ErrValidation), errors.Is(err, store.ErrValidation):
		WriteError(w, http.StatusBadRequest, "validation_failed", err.Error(), nil)
	default:
		WriteError(w, http.StatusInternalServerError, "internal_error", err.Error(), nil)
	}
}
