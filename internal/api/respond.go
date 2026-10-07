package api

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/provider"
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
		serverError(w, op, err, failMsg)
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
// maxRequestBodyBytes; see decodeJSONLimit.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	return decodeJSONLimit(w, r, v, maxRequestBodyBytes)
}

// decodeJSONLimit decodes the request body into v, refusing a body over
// limit bytes and, as a 400 naming it, any field v doesn't have. Every route
// with a body decodes this way: a field a client misspells would otherwise
// save as its zero value (a misspelled focus_glow_enabled saves false), and a
// push sent in another shape by a tab loaded before the shape changed would
// read as an empty selection and take every Uno collection off Nuvio. An
// oversized body is its own status (413) rather than a generic 400, so a
// client can tell "too big" from "malformed".
func decodeJSONLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return decoded(w, d, v)
}

// unknownFieldPrefix starts the error a strict decoder gives for a field v
// doesn't have: json: unknown field "name".
const unknownFieldPrefix = "json: unknown field "

// decoded decodes d into v, answering a failure itself: 413 for a body over
// its limit, else 400. A strict decoder's unknown field is named in the 400.
func decoded(w http.ResponseWriter, d *json.Decoder, v any) bool {
	if err := d.Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		httpx.WriteError(w, http.StatusBadRequest, badBodyMessage(err))
		return false
	}
	return true
}

// badBodyMessage is the 400 text for a body that failed to decode: the
// fixed refusal, naming the field when a strict decoder met one v lacks.
func badBodyMessage(err error) string {
	if field, ok := strings.CutPrefix(err.Error(), unknownFieldPrefix); ok {
		return "invalid request body: unknown field " + field
	}
	return "invalid request body"
}

// writeVaultError classifies a vault-layer (or vault-flavored, see
// validateCatalogParams) error into one HTTP response: an error the caller
// can act on (clientErrors) is its 422, 400, 403 or 409 with the row's words,
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
// judged.
func writeVaultError(w http.ResponseWriter, op string, err error, notFound error, notFoundMsg, defaultMsg string) {
	switch status, msg := clientFailureOf(clientErrors, err); {
	case status != 0:
		httpx.WriteError(w, status, msg)
	case notFound != nil && errors.Is(err, notFound):
		httpx.WriteError(w, http.StatusNotFound, notFoundMsg)
	case errors.Is(err, errUpstreamValidation):
		log.Printf("%s: %v", op, err)
		httpx.WriteError(w, http.StatusBadGateway, "failed to reach TMDB")
	default:
		serverError(w, op, err, defaultMsg)
	}
}

// clientFailure pairs an error the caller can act on with the status it is
// answered with, and the words: message, or the error's own when message is
// empty.
type clientFailure struct {
	sentinel error
	status   int
	message  string
}

// clientFailureOf is the status and words table gives err, or 0 when err is
// in none of its rows. Rows are tried in order.
func clientFailureOf(table []clientFailure, err error) (int, string) {
	for _, f := range table {
		if errors.Is(err, f.sentinel) {
			return f.status, cmp.Or(f.message, err.Error())
		}
	}
	return 0, ""
}

// keyFailures answer a TMDB call the account's own key couldn't make, on a
// server where each account brings one: it has saved none, or TMDB doesn't
// accept it. They are a 422, a status nothing else in the builder API
// answers, so the SPA can tell a key problem from any other failure: not a
// 401 (the SPA refreshes and retries on one), a 403 (the access refusal) or a
// 409 (Already added).
var keyFailures = []clientFailure{
	{provider.ErrNoKey, http.StatusUnprocessableEntity, "This account has no TMDB key yet."},
	{provider.ErrKeyRejected, http.StatusUnprocessableEntity, "TMDB didn't accept your key."},
}

// clientErrors are the errors a vault write or its recipe check answers
// the caller with: a key problem (keyFailures), then 400 for
// vault.ErrInvalidInput and 409 for vault.ErrConflict, each in the error's own
// words — safe, since every such message is built from validation or state
// text, never a lower-level detail.
var clientErrors = slices.Concat(keyFailures, []clientFailure{
	{vault.ErrInvalidInput, http.StatusBadRequest, ""},
	{vault.ErrConflict, http.StatusConflict, ""},
})

// The codes of the errors the SPA acts on by kind rather than by status
// (httpx.ErrorBody.Code).
const (
	// codeProfileNotFound: the caller has no Uno profile in the URL's slot, so
	// it was never selected (requireProfile).
	codeProfileNotFound = "profile_not_found"
)

// serverError logs err under op and answers a 500 with msg, which says what
// failed and nothing of why.
func serverError(w http.ResponseWriter, op string, err error, msg string) {
	log.Printf("%s: %v", op, err)
	httpx.WriteError(w, http.StatusInternalServerError, msg)
}

// writeNuvioError classifies a Nuvio-call error and writes an error
// answer: nuvio.ErrNuvioRequestFailed is upstream's fault (502, "nuvio
// unavailable"), anything else is ours (500, defaultMsg). Delegates the
// classification to nuvioErrorStatus (push.go) so push's answers and
// these can't drift apart on what counts as an upstream
// failure.
func writeNuvioError(w http.ResponseWriter, err error, defaultMsg string) {
	if nuvioErrorStatus(err) == http.StatusBadGateway {
		httpx.WriteError(w, http.StatusBadGateway, "nuvio unavailable")
		return
	}
	httpx.WriteError(w, http.StatusInternalServerError, defaultMsg)
}
