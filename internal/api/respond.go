package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/hiidz/uno/internal/vault"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

// writeVaultError classifies a vault-layer (or vault-flavored, see
// validateCatalogParams) error into one HTTP response: vault.ErrInvalidInput
// is a 400 using the error's own message (safe — every ErrInvalidInput
// message is built from validation text, never a lower-level detail),
// notFound is the resource's own not-found sentinel (pass nil to skip that
// case, e.g. create has none), and anything else is logged under op and
// answered with defaultMsg as a 500. Shared by every catalog/collection
// create/update/delete handler so both resources fail the same way — before
// this, catalogs answered a bad recipe with a bare http.Error(err.Error(),
// 400) from validateCatalogParams while a bad vault.CatalogForm went through
// this same errors.Is switch; collections only ever had the latter.
func writeVaultError(w http.ResponseWriter, op string, err error, notFound error, notFoundMsg, defaultMsg string) {
	switch {
	case errors.Is(err, vault.ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case notFound != nil && errors.Is(err, notFound):
		http.Error(w, notFoundMsg, http.StatusNotFound)
	default:
		log.Printf("%s: %v", op, err)
		http.Error(w, defaultMsg, http.StatusInternalServerError)
	}
}

// writeNuvioError classifies a Nuvio-call error and writes a plain-text
// response: nuvio.ErrNuvioRequestFailed is upstream's fault (502, "nuvio
// unavailable"), anything else is ours (500, defaultMsg). Delegates the
// classification to nuvioErrorStatus (push.go) so its JSON responses and
// these plain-text ones can't drift apart on what counts as an upstream
// failure — profiles.go used to hand-roll this same errors.Is check twice.
func writeNuvioError(w http.ResponseWriter, err error, defaultMsg string) {
	if nuvioErrorStatus(err) == http.StatusBadGateway {
		http.Error(w, "nuvio unavailable", http.StatusBadGateway)
		return
	}
	http.Error(w, defaultMsg, http.StatusInternalServerError)
}
