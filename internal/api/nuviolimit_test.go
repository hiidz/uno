package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/provider"
)

// tokenIsSub authenticates every bearer token as the account the token names.
type tokenIsSub struct{}

func (tokenIsSub) Verify(_ context.Context, token string) (nuvio.Claims, error) {
	return nuvio.Claims{Sub: token}, nil
}

// An account gets nuvioCallBurst requests to the routes that call Nuvio, then
// a 429 with the seconds to wait, before Nuvio is called; another account's
// budget is its own, and once a refill has passed the next request goes
// through.
func TestNuvioCallsArePacedPerAccount(t *testing.T) {
	fake := &fakeNuvio{}
	s, err := New(Deps{
		Vault:        newTestVaultDB(t),
		Provider:     provider.NewTMDBClient("key"),
		Verifier:     tokenIsSub{},
		Nuvio:        fake,
		SiteBaseURL:  "http://example.com",
		NuvioBaseURL: "https://nuvio.example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Unix(1_000_000, 0)
	s.nuvioCalls.now = func() time.Time { return now }

	listProfiles := func(account string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/profiles", nil)
		req.Header.Set("Authorization", "Bearer "+account)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		return w
	}

	for i := range nuvioCallBurst {
		if w := listProfiles("flooder"); w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (body %q)", i+1, w.Code, w.Body.String())
		}
	}
	calls := fake.listProfilesCalls

	w := listProfiles("flooder")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("request %d: status = %d, want 429", nuvioCallBurst+1, w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "3" {
		t.Errorf("Retry-After = %q, want 3", got)
	}
	if fake.listProfilesCalls != calls {
		t.Error("a refused request still called Nuvio")
	}

	select2 := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/profiles/select", nil)
	select2.Header.Set("Authorization", "Bearer flooder")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, select2)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("profile select over the budget: status = %d, want 429: the routes share one budget", w.Code)
	}

	if w := listProfiles("bystander"); w.Code != http.StatusOK {
		t.Errorf("another account: status = %d, want 200", w.Code)
	}

	now = now.Add(nuvioCallEvery)
	if w := listProfiles("flooder"); w.Code != http.StatusOK {
		t.Errorf("after one refill: status = %d, want 200", w.Code)
	}
	if w := listProfiles("flooder"); w.Code != http.StatusTooManyRequests {
		t.Errorf("right after the refill was spent: status = %d, want 429", w.Code)
	}
}

// Retry-After is the wait rounded up to whole seconds, never 0: a client
// that waits that long is let through.
func TestRetryAfterSeconds(t *testing.T) {
	for _, tc := range []struct {
		wait time.Duration
		want int
	}{
		{time.Millisecond, 1},
		{time.Second, 1},
		{2100 * time.Millisecond, 3},
		{3 * time.Second, 3},
	} {
		if got := retryAfterSeconds(tc.wait); got != tc.want {
			t.Errorf("retryAfterSeconds(%v) = %d, want %d", tc.wait, got, tc.want)
		}
	}
}

// A bucket that has refilled is dropped by the next sweep; one still
// refilling stays.
func TestAccountLimiterSweepsRefilledBuckets(t *testing.T) {
	l := newAccountLimiter(2, time.Second)
	now := time.Unix(1_000_000, 0)
	l.now = func() time.Time { return now }

	l.allow("quiet")
	now = now.Add(accountLimiterSweep)
	l.allow("busy")
	l.allow("busy")

	if _, ok := l.byAccount["quiet"]; ok {
		t.Error("a refilled bucket survived the sweep")
	}
	if _, ok := l.byAccount["busy"]; !ok {
		t.Error("a bucket still refilling was dropped")
	}
}
