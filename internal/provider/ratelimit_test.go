package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The bucket lets burst requests out at once, then one per 1/rate seconds,
// and a pause holds every request until it ends.
func TestLimiterReserve(t *testing.T) {
	l := newLimiter(1, 2)
	t0 := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	for i, want := range []time.Duration{0, 0, time.Second} {
		if got := l.reserve(t0); got != want {
			t.Errorf("reserve %d at t0 = %v, want %v", i, got, want)
		}
	}
	if got := l.reserve(t0.Add(time.Second)); got != 0 {
		t.Errorf("reserve a second later = %v, want 0", got)
	}

	l.pause(t0.Add(time.Second), 5*time.Second)
	l.pause(t0.Add(time.Second), time.Second)
	if got := l.reserve(t0.Add(2 * time.Second)); got != 4*time.Second {
		t.Errorf("reserve during the pause = %v, want the 4s left of the longer one", got)
	}
	if got := l.reserve(t0.Add(6 * time.Second)); got != 0 {
		t.Errorf("reserve after the pause = %v, want 0", got)
	}
}

// wait gives up when its context ends before the limiter lets it out.
func TestLimiterWaitEndsWithItsContext(t *testing.T) {
	l := newLimiter(1, 1)
	l.pause(time.Now(), time.Hour)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if err := l.wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("wait = %v, want the context's deadline", err)
	}
}

// Requests past the burst go out at the limiter's rate.
func TestGetIsPaced(t *testing.T) {
	c, _ := statusSequenceTMDB(t)
	c.limiter = newLimiter(50, 1)
	start := time.Now()
	for range 6 {
		if err := c.get(t.Context(), "/configuration/countries", nil, &[]Country{}); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 90*time.Millisecond {
		t.Errorf("6 requests at 50/s with a burst of 1 took %v, want at least 100ms", elapsed)
	}
}

// retryAfter reads seconds or an HTTP date, clamps to [0, maxRetryAfter],
// and falls back to defaultRetryAfter.
func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	for header, want := range map[string]time.Duration{
		"3":                             3 * time.Second,
		"0":                             0,
		"-5":                            0,
		"100000":                        maxRetryAfter,
		"99999999999":                   maxRetryAfter,
		"-99999999999":                  0,
		"":                              defaultRetryAfter,
		"soon":                          defaultRetryAfter,
		"Tue, 29 Sep 2026 12:00:05 GMT": 5 * time.Second,
		"Tue, 29 Sep 2026 13:00:00 GMT": maxRetryAfter,
	} {
		if got := retryAfter(header, now); got != want {
			t.Errorf("retryAfter(%q) = %v, want %v", header, got, want)
		}
	}
}

// statusSequenceTMDB answers each request with the next of statuses, a
// country list once they run out, and counts the requests.
func statusSequenceTMDB(t *testing.T, statuses ...int) (*TMDBClient, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := int(requests.Add(1))
		if n <= len(statuses) {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(statuses[n-1])
			return
		}
		fmt.Fprint(w, `[{"iso_3166_1":"GB","english_name":"United Kingdom"}]`)
	}))
	t.Cleanup(srv.Close)
	c := NewTMDBClient("key")
	c.baseURL = srv.URL
	return c, &requests
}

// A 429 pauses the limiter for its Retry-After and the request goes once
// more; a second 429 is the answer.
func TestGetRetriesATooManyRequestsOnce(t *testing.T) {
	c, requests := statusSequenceTMDB(t, http.StatusTooManyRequests)
	var countries []Country
	if err := c.get(t.Context(), "/configuration/countries", nil, &countries); err != nil || len(countries) != 1 {
		t.Fatalf("get after one 429 = %v, %v; want the list", countries, err)
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}
	if c.limiter.blocked.IsZero() {
		t.Error("the 429 didn't pause the limiter")
	}

	c, requests = statusSequenceTMDB(t, http.StatusTooManyRequests, http.StatusTooManyRequests)
	if err := c.get(t.Context(), "/configuration/countries", nil, &countries); err == nil || !strings.Contains(err.Error(), "status 429") {
		t.Errorf("get after two 429s = %v, want the 429", err)
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}
}
