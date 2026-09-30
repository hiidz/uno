package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/provider"
)

// tokenIsEmail is a TokenVerifier whose account's email address is the token
// itself, so a test can send requests as several accounts. DevBypassSub and
// "no-email" name accounts whose tokens carry no email. The tokens "expired"
// and "jwks-down" fail verification the way Nuvio's would.
type tokenIsEmail struct{}

func (tokenIsEmail) Verify(_ context.Context, token string) (nuvio.Claims, error) {
	switch token {
	case "expired":
		return nuvio.Claims{}, fmt.Errorf("%w: expired", nuvio.ErrInvalidToken)
	case "jwks-down":
		return nuvio.Claims{}, fmt.Errorf("%w: dial tcp", nuvio.ErrJWKSUnavailable)
	case "odd":
		return nuvio.Claims{}, fmt.Errorf("something else")
	case DevBypassSub, "no-email":
		return nuvio.Claims{Sub: token}, nil
	}
	return nuvio.Claims{Sub: "sub-" + token, Email: token}, nil
}

// newAccessTestServer builds a Server whose accounts' emails are their tokens,
// under access.
func newAccessTestServer(t *testing.T, access Access) *Server {
	t.Helper()
	s, err := New(Deps{
		Vault:        newTestVaultDB(t),
		Provider:     provider.NewTMDBClient("key"),
		Verifier:     tokenIsEmail{},
		Nuvio:        &fakeNuvio{profiles: []nuvio.NuvioProfile{}},
		SiteBaseURL:  "http://example.com",
		NuvioBaseURL: "https://nuvio.example.com",
		Access:       access,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// serveAs sends one GET through s's router with authorization as its
// Authorization header, none when empty.
func serveAs(t *testing.T, s *Server, authorization, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w
}

// Under an allowlist, a verified account whose email is listed is admitted,
// in any case. One that isn't listed, or whose token carries no email, is a
// 403 on every authenticated route, profile-scoped ones included, never a 401
// the SPA would refresh and retry. A token that fails verification is still a
// 401 (or a 502 when Nuvio's keys can't be fetched), whoever it names.
func TestAccessAllowlist(t *testing.T) {
	s := newAccessTestServer(t, Access{Allowlist: true, Emails: []string{"allowed@example.com"}})
	for _, tc := range []struct {
		name, authorization, path string
		wantStatus                int
		wantBody                  string
	}{
		{"a listed email", "Bearer allowed@example.com", "/api/profiles", http.StatusOK, "[]"},
		{"a listed email in another case", "Bearer Allowed@Example.COM", "/api/profiles", http.StatusOK, "[]"},
		{"another email", "Bearer stranger@example.com", "/api/profiles", http.StatusForbidden, "can't use this Uno server"},
		{"a token without an email", "Bearer no-email", "/api/profiles", http.StatusForbidden, "can't use this Uno server"},
		{"the dev bypass's account, not admitted", "Bearer " + DevBypassSub, "/api/profiles", http.StatusForbidden, "can't use this Uno server"},
		{"another email on a profile route", "Bearer stranger@example.com", "/api/p/1/catalogs", http.StatusForbidden, "can't use this Uno server"},
		{"another email on a lookup", "Bearer stranger@example.com", "/api/languages", http.StatusForbidden, "can't use this Uno server"},
		{"no header", "", "/api/profiles", http.StatusUnauthorized, "missing or malformed"},
		{"not a bearer token", "Basic allowed@example.com", "/api/profiles", http.StatusUnauthorized, "missing or malformed"},
		{"an empty bearer token", "Bearer ", "/api/profiles", http.StatusUnauthorized, "missing or malformed"},
		{"an expired token", "Bearer expired", "/api/profiles", http.StatusUnauthorized, "invalid or expired token"},
		{"Nuvio's keys out of reach", "Bearer jwks-down", "/api/profiles", http.StatusBadGateway, "auth service unavailable"},
		{"any other verify failure", "Bearer odd", "/api/profiles", http.StatusUnauthorized, "authentication failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := serveAs(t, s, tc.authorization, tc.path)
			if w.Code != tc.wantStatus || !strings.Contains(w.Body.String(), tc.wantBody) {
				t.Fatalf("answer = %d %q, want %d containing %q", w.Code, w.Body.String(), tc.wantStatus, tc.wantBody)
			}
		})
	}
}

// The zero Access admits every verified account, one without an email too.
func TestAccessOpen(t *testing.T) {
	s := newAccessTestServer(t, Access{})
	for _, account := range []string{"anyone@example.com", "no-email"} {
		if w := serveAs(t, s, "Bearer "+account, "/api/profiles"); w.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 (body %q)", account, w.Code, w.Body.String())
		}
	}
}

// WithDevBypass admits the bypass's fake account by its id besides the
// listed emails, leaving the Access it was called on as it was.
func TestAccessWithDevBypass(t *testing.T) {
	listed := Access{Allowlist: true, Emails: []string{"allowed@example.com"}}
	s := newAccessTestServer(t, listed.WithDevBypass())
	for _, account := range []string{"allowed@example.com", DevBypassSub} {
		if w := serveAs(t, s, "Bearer "+account, "/api/profiles"); w.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", account, w.Code)
		}
	}
	for _, account := range []string{"stranger@example.com", "no-email"} {
		if w := serveAs(t, s, "Bearer "+account, "/api/profiles"); w.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", account, w.Code)
		}
	}
	if listed.DevBypass {
		t.Error("WithDevBypass changed the Access it was called on")
	}
}
