package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hiidz/uno/internal/vault"
)

// TestWriteVaultErrorClassification pins the one contract every catalog,
// collection and preview handler shares: which failure becomes which
// status, and what of the error reaches the wire. The upstream case is the
// reason this switch has four branches — a recipe that couldn't be checked
// against TMDB must not be reported as the caller's bad input, nor as Uno
// failing to save.
func TestWriteVaultErrorClassification(t *testing.T) {
	errNotFound := errors.New("catalog not found sentinel")

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "rejected recipe is the caller's fault",
			err:        fmt.Errorf("%w: with_genres entry \"999\" is not a TMDB genre id", vault.ErrInvalidInput),
			wantStatus: http.StatusBadRequest,
			wantBody:   "is not a TMDB genre id",
		},
		{
			name:       "state the caller's request conflicts with is a 409",
			err:        fmt.Errorf("%w: already taken", vault.ErrConflict),
			wantStatus: http.StatusConflict,
			wantBody:   "already taken",
		},
		{
			name:       "missing row is a 404",
			err:        fmt.Errorf("loading: %w", errNotFound),
			wantStatus: http.StatusNotFound,
			wantBody:   "catalog not found",
		},
		{
			name:       "unreachable TMDB is upstream's fault",
			err:        fmt.Errorf("%w: %w", errUpstreamValidation, errors.New("provider: fetch /configuration/languages: dial tcp: refused")),
			wantStatus: http.StatusBadGateway,
			wantBody:   "failed to reach TMDB",
		},
		{
			name:       "anything else is ours",
			err:        errors.New("database is locked"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "failed to save catalog",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeVaultError(w, "testOp", tc.err, errNotFound, "catalog not found", "failed to save catalog")

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if body := w.Body.String(); !strings.Contains(body, tc.wantBody) {
				t.Fatalf("body = %q, want it to contain %q", strings.TrimSpace(body), tc.wantBody)
			}
		})
	}
}

// TestWriteVaultErrorHidesUpstreamDetail keeps the 502's body free of the
// underlying transport error: it is logged, not served.
func TestWriteVaultErrorHidesUpstreamDetail(t *testing.T) {
	w := httptest.NewRecorder()
	writeVaultError(w, "testOp",
		fmt.Errorf("%w: %w", errUpstreamValidation, errors.New("dial tcp 10.0.0.1:443: connect: refused")),
		nil, "", "failed to save catalog")

	if body := w.Body.String(); strings.Contains(body, "10.0.0.1") {
		t.Fatalf("502 body leaked the transport error: %q", strings.TrimSpace(body))
	}
}
