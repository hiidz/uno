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
		http.Error(w, "invalid "+label, http.StatusBadRequest)
		return uuid.UUID{}, false
	}
	return id, true
}
