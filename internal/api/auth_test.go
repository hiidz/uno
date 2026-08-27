package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hiidz/uno/internal/nuvio"
)

// ---------------------------------------------------------------------------
// fakeVerifier: exercises TokenVerifier without a real Nuvio JWKS fetch.
// ---------------------------------------------------------------------------

type fakeVerifier struct {
	claims       nuvio.Claims
	err          error
	receivedTok  string // the token Verify was actually called with
	verifyCalled bool
}

func (f *fakeVerifier) Verify(ctx context.Context, token string) (nuvio.Claims, error) {
	f.verifyCalled = true
	f.receivedTok = token
	return f.claims, f.err
}

func TestRequireNuvioAuth(t *testing.T) {
	tests := []struct {
		name             string
		authHeader       string
		fake             *fakeVerifier
		wantStatus       int
		wantNextCalled   bool
		wantVerifyCalled bool // false only for the three pre-verify short-circuits below
	}{
		{
			name:             "missing Authorization header",
			authHeader:       "",
			fake:             &fakeVerifier{},
			wantStatus:       http.StatusUnauthorized,
			wantNextCalled:   false,
			wantVerifyCalled: false,
		},
		{
			name:             "header without Bearer prefix",
			authHeader:       "Token abc123",
			fake:             &fakeVerifier{},
			wantStatus:       http.StatusUnauthorized,
			wantNextCalled:   false,
			wantVerifyCalled: false,
		},
		{
			name:             "Bearer with empty token",
			authHeader:       "Bearer ",
			fake:             &fakeVerifier{},
			wantStatus:       http.StatusUnauthorized,
			wantNextCalled:   false,
			wantVerifyCalled: false,
		},
		{
			name:             "verifier returns ErrInvalidToken",
			authHeader:       "Bearer sometoken",
			fake:             &fakeVerifier{err: nuvio.ErrInvalidToken},
			wantStatus:       http.StatusUnauthorized,
			wantNextCalled:   false,
			wantVerifyCalled: true,
		},
		{
			name:             "verifier returns ErrJWKSUnavailable",
			authHeader:       "Bearer sometoken",
			fake:             &fakeVerifier{err: nuvio.ErrJWKSUnavailable},
			wantStatus:       http.StatusBadGateway,
			wantNextCalled:   false,
			wantVerifyCalled: true,
		},
		{
			name:             "verifier returns an unclassified error",
			authHeader:       "Bearer sometoken",
			fake:             &fakeVerifier{err: errors.New("boom")},
			wantStatus:       http.StatusUnauthorized,
			wantNextCalled:   false,
			wantVerifyCalled: true,
		},
		{
			name:       "success",
			authHeader: "Bearer sometoken",
			fake: &fakeVerifier{
				claims: nuvio.Claims{Sub: "user-123", Exp: time.Now().Add(time.Hour)},
			},
			wantStatus:       http.StatusOK,
			wantNextCalled:   true,
			wantVerifyCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{verifier: tt.fake}

			var nextCalled bool
			var gotSub, gotToken string
			var gotSubOK, gotTokenOK bool
			next := func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				gotSub, gotSubOK = nuvioUserIDFrom(r.Context())
				gotToken, gotTokenOK = nuvioTokenFrom(r.Context())
				w.WriteHeader(http.StatusOK)
			}

			req := httptest.NewRequest(http.MethodGet, "/api/profiles", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()

			s.requireNuvioAuth(next)(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if nextCalled != tt.wantNextCalled {
				t.Errorf("next called = %v, want %v", nextCalled, tt.wantNextCalled)
			}
			if tt.fake.verifyCalled != tt.wantVerifyCalled {
				t.Errorf("verifier.Verify called = %v, want %v", tt.fake.verifyCalled, tt.wantVerifyCalled)
			}

			if tt.wantNextCalled {
				if !gotSubOK || gotSub != tt.fake.claims.Sub {
					t.Errorf("nuvioUserIDFrom(ctx) = %q, %v; want %q, true", gotSub, gotSubOK, tt.fake.claims.Sub)
				}
				// The token reaching the handler's context, and the token the
				// verifier was called with, must both be the Bearer-stripped
				// value — never the raw header.
				wantToken := "sometoken"
				if !gotTokenOK || gotToken != wantToken {
					t.Errorf("nuvioTokenFrom(ctx) = %q, %v; want %q, true", gotToken, gotTokenOK, wantToken)
				}
				if tt.fake.receivedTok != wantToken {
					t.Errorf("verifier received token %q, want %q", tt.fake.receivedTok, wantToken)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// fakeNuvioClient: exercises NuvioClient without a real Nuvio RPC client.
// Only ListProfiles is exercised by TestListProfiles below; the other four
// methods exist solely to satisfy the interface.
// ---------------------------------------------------------------------------

type fakeNuvioClient struct {
	profiles []nuvio.NuvioProfile
	err      error
}

func (f *fakeNuvioClient) ListProfiles(ctx context.Context, accessToken string) ([]nuvio.NuvioProfile, error) {
	return f.profiles, f.err
}

func (f *fakeNuvioClient) ListAddons(ctx context.Context, accessToken string, profileID int) ([]nuvio.NuvioAddon, error) {
	return nil, nil
}

func (f *fakeNuvioClient) PushAddons(ctx context.Context, accessToken string, profileID int, addons []nuvio.PushAddonInput) error {
	return nil
}

func (f *fakeNuvioClient) PullCollections(ctx context.Context, accessToken string, profileID int) ([]json.RawMessage, error) {
	return nil, nil
}

func (f *fakeNuvioClient) PushCollections(ctx context.Context, accessToken string, profileID int, collections []json.RawMessage) error {
	return nil
}

func TestListProfiles(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fake := &fakeNuvioClient{
			profiles: []nuvio.NuvioProfile{
				{ID: "p1", UserID: "u1", ProfileIndex: 1, Name: "Alice"},
			},
		}
		s := &Server{nuvio: fake}

		req := httptest.NewRequest(http.MethodGet, "/api/profiles", nil)
		req = req.WithContext(withNuvioToken(req.Context(), "sometoken"))
		rec := httptest.NewRecorder()

		s.listProfiles(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		var got []nuvio.NuvioProfile
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decoding response body: %v", err)
		}
		if len(got) != 1 || got[0].Name != "Alice" {
			t.Errorf("body = %+v, want the fake's profile list", got)
		}
	})

	t.Run("nuvio request failed", func(t *testing.T) {
		fake := &fakeNuvioClient{err: nuvio.ErrNuvioRequestFailed}
		s := &Server{nuvio: fake}

		req := httptest.NewRequest(http.MethodGet, "/api/profiles", nil)
		req = req.WithContext(withNuvioToken(req.Context(), "sometoken"))
		rec := httptest.NewRecorder()

		s.listProfiles(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
		}
	})
}
