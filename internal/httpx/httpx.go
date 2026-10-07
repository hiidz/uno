// Package httpx holds the request and response plumbing Uno's two HTTP
// surfaces share: the authenticated builder API (internal/api) and the
// public Stremio addon server (internal/addon).
package httpx

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/google/uuid"
)

// WriteJSON writes v as the response body under status. An encoding failure
// is logged rather than returned: the status line and headers are already on
// the wire by then, so there is nothing left to tell the client.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

// PathUUID parses the request's {param} path segment as a UUID. A malformed
// value answers 400 with "invalid <label>" and reports false, so the caller
// returns without writing anything more. label is spelled out rather than
// derived from param so the message stays prose ("catalog id", not
// "catalogID").
func PathUUID(w http.ResponseWriter, r *http.Request, param, label string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(param))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid "+label)
		return uuid.UUID{}, false
	}
	return id, true
}

// ErrorBody is the JSON body of every error the builder API answers: Error is
// its words and Code, when it has one, a stable name for an error the caller
// acts on by kind rather than by status.
type ErrorBody struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// WriteError answers status with msg as an ErrorBody.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, ErrorBody{Error: msg})
}

// WriteCodedError is WriteError for an error with a stable code.
func WriteCodedError(w http.ResponseWriter, status int, msg, code string) {
	WriteJSON(w, status, ErrorBody{Error: msg, Code: code})
}
