package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/benjaminbenny/internal-transfers-system/internal/service"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func writeInvalidJSON(w http.ResponseWriter) {
	writeError(w, http.StatusBadRequest, "invalid_json")
}

func mapError(err error) (status int, code string) {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		return http.StatusBadRequest, "invalid_input"
	case errors.Is(err, service.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, service.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, service.ErrInsufficientFunds):
		return http.StatusConflict, "insufficient_funds"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}
