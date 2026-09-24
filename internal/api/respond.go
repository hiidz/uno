package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/vault"
)

// listByProfile answers with everything load returns for the request's
// profile. op names the handler in the 500's log line, failMsg is what the
// client sees instead of the error. Must run behind requireProfile, which is
// what makes the profile id in the context a given.
func listByProfile[T any](w http.ResponseWriter, r *http.Request, op, failMsg string, load func(context.Context, uuid.UUID) ([]T, error)) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	items, err := load(r.Context(), profileID)
	if err != nil {
		log.Printf("%s: %v", op, err)
		http.Error(w, failMsg, http.StatusInternalServerError)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

// maxRequestBodyBytes caps every JSON request body the builder API accepts.
// The largest legitimate body is a collection save or a push carrying a
// whole selection, both orders of magnitude under this; anything larger is
// a client bug or an attempt to make the server buffer for free.
const maxRequestBodyBytes = 1 << 20 // 1 MiB

// decodeJSON decodes the request body into v, refusing a body over
// maxRequestBodyBytes. An oversized body is its own status (413) rather than
// a generic 400, so a client can tell "too big" from "malformed".
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return false
		}
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

// writeVaultError classifies a vault-layer (or vault-flavored, see
// validateCatalogParams) error into one HTTP response: an error the caller
// caused (clientErrorStatus) is its 400 or 409 using the error's own message
// (safe — every ErrInvalidInput and ErrConflict message is built from
// validation or state text, never a lower-level detail),
// notFound is the resource's own not-found sentinel (pass nil to skip that
// case, e.g. create has none), errUpstreamValidation is a 502 because a
// recipe that couldn't be checked against TMDB has not been found at fault,
// and anything else is logged under op and answered with defaultMsg as a
// 500. The upstream case carries a fixed message rather than defaultMsg:
// every caller would otherwise phrase the same TMDB outage as its own
// failure ("failed to create catalog"), which is the one thing it isn't.
//
// Shared by every catalog/collection create/update/delete handler and by
// the two preview routes, so a recipe fails the same way wherever it is
// judged — before this, catalogs answered a bad recipe with a bare
// http.Error(err.Error(), 400) from validateCatalogParams while a bad
// vault.CatalogForm went through this same errors.Is switch.
func writeVaultError(w http.ResponseWriter, op string, err error, notFound error, notFoundMsg, defaultMsg string) {
	switch status := clientErrorStatus(err); {
	case status != 0:
		http.Error(w, err.Error(), status)
	case notFound != nil && errors.Is(err, notFound):
		http.Error(w, notFoundMsg, http.StatusNotFound)
	case errors.Is(err, errUpstreamValidation):
		log.Printf("%s: %v", op, err)
		http.Error(w, "failed to reach TMDB", http.StatusBadGateway)
	default:
		log.Printf("%s: %v", op, err)
		http.Error(w, defaultMsg, http.StatusInternalServerError)
	}
}

// clientErrorStatus is the status of a vault error the caller caused —
// 400 for vault.ErrInvalidInput, 409 for vault.ErrConflict — or 0 for any
// other error.
func clientErrorStatus(err error) int {
	switch {
	case errors.Is(err, vault.ErrInvalidInput):
		return http.StatusBadRequest
	case errors.Is(err, vault.ErrConflict):
		return http.StatusConflict
	}
	return 0
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
