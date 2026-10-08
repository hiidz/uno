package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// keyedTMDB is a TMDB that answers 401 to any api_key but good ones, and
// records the key of each call.
func keyedTMDB(t *testing.T, shared string, good ...string) (*TMDBClient, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var keys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("api_key")
		mu.Lock()
		keys = append(keys, key)
		mu.Unlock()
		for _, g := range good {
			if key == g {
				w.Write([]byte(`{"success":true}`))
				return
			}
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	c := NewTMDBClient(shared)
	c.baseURL = srv.URL
	return c, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), keys...)
	}
}

func source(key string, err error) KeySource {
	return func() (string, error) { return key, err }
}

// A call uses the account's key from its context, else the shared one; with
// neither it is ErrNoKey, and so is an account that has saved none, without
// reaching TMDB.
func TestGetUsesTheCallsKey(t *testing.T) {
	var out struct{}
	c, keys := keyedTMDB(t, "shared", "shared", "mine")
	if err := c.get(t.Context(), "/x", nil, &out); err != nil {
		t.Fatalf("shared: %v", err)
	}
	if err := c.get(WithKeySource(t.Context(), source("mine", nil)), "/x", nil, &out); err != nil {
		t.Fatalf("own: %v", err)
	}
	if err := c.get(WithKeySource(t.Context(), source("", ErrNoKey)), "/x", nil, &out); !errors.Is(err, ErrNoKey) {
		t.Errorf("an account with no key: %v, want ErrNoKey", err)
	}
	if err := c.get(WithKeySource(t.Context(), nil), "/x", nil, &out); err != nil {
		t.Errorf("a nil source: %v, want the shared key used", err)
	}
	if got := strings.Join(keys(), ","); got != "shared,mine,shared" {
		t.Errorf("keys sent = %s, want shared,mine,shared", got)
	}

	perAccount, keys := keyedTMDB(t, "")
	if err := perAccount.get(t.Context(), "/x", nil, &out); !errors.Is(err, ErrNoKey) {
		t.Errorf("no shared key and none in the context: %v, want ErrNoKey", err)
	}
	if len(keys()) != 0 {
		t.Errorf("TMDB was reached %d times, want none", len(keys()))
	}
}

// A key source runs once however many calls the request makes, and not at
// all for a request that makes none.
func TestKeySourceRunsOnce(t *testing.T) {
	c, _ := keyedTMDB(t, "", "mine")
	var reads atomic.Int32
	ctx := WithKeySource(t.Context(), func() (string, error) {
		reads.Add(1)
		return "mine", nil
	})
	var out struct{}
	for range 3 {
		if err := c.get(ctx, "/x", nil, &out); err != nil {
			t.Fatal(err)
		}
	}
	WithKeySource(t.Context(), func() (string, error) {
		t.Error("an unused source ran")
		return "", nil
	})
	if got := reads.Load(); got != 1 {
		t.Errorf("reads = %d, want 1", got)
	}
}

// A 401 on an account's own key is ErrKeyRejected; on the shared key it is
// an ordinary TMDB failure, the operator's to fix.
func TestUnauthorizedIsKeyRejectedOnlyForAnAccountsKey(t *testing.T) {
	c, _ := keyedTMDB(t, "shared-bad")
	var out struct{}
	if err := c.get(WithKeySource(t.Context(), source("bad", nil)), "/x", nil, &out); !errors.Is(err, ErrKeyRejected) || !IsKeyError(err) {
		t.Errorf("own key: %v, want ErrKeyRejected", err)
	}
	err := c.get(t.Context(), "/x", nil, &out)
	if err == nil || IsKeyError(err) || !strings.Contains(err.Error(), "status 401") {
		t.Errorf("shared key: %v, want a plain status error", err)
	}
}

// CheckKey asks TMDB with the candidate key alone.
func TestCheckKey(t *testing.T) {
	c, keys := keyedTMDB(t, "shared", "shared", "good")
	if err := c.CheckKey(t.Context(), "good"); err != nil {
		t.Errorf("a good key: %v", err)
	}
	if err := c.CheckKey(t.Context(), "bad"); !errors.Is(err, ErrKeyRejected) {
		t.Errorf("a bad key: %v, want ErrKeyRejected", err)
	}
	if got := strings.Join(keys(), ","); got != "good,bad" {
		t.Errorf("keys sent = %s, want good,bad", got)
	}
}

// A request that fails before TMDB answers never carries the key into its
// error, which is logged.
func TestFailedRequestErrorHoldsNoKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()
	c := NewTMDBClient("shared-secret-key")
	c.baseURL = srv.URL
	var out struct{}
	for _, ctx := range []context.Context{t.Context(), WithKeySource(t.Context(), source("account-secret-key", nil))} {
		err := c.get(ctx, "/x", nil, &out)
		if err == nil {
			t.Fatal("get against a closed server succeeded")
		}
		if strings.Contains(err.Error(), "secret-key") || strings.Contains(err.Error(), "api_key") {
			t.Errorf("error %q carries the key", err)
		}
	}
}

// An account's key waits on its own limiter as well as the shared one; the
// shared key waits on the shared one only.
func TestAccountKeysArePacedOnTheirOwn(t *testing.T) {
	c, _ := keyedTMDB(t, "shared", "shared", "mine")
	c.keyLimiters = newKeyLimiters(50, 1)
	var out struct{}
	start := time.Now()
	for range 6 {
		if err := c.get(t.Context(), "/x", nil, &out); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed > 80*time.Millisecond {
		t.Errorf("six calls on the shared key took %v, want no per-key pacing", elapsed)
	}
	ctx := WithKeySource(t.Context(), source("mine", nil))
	start = time.Now()
	for range 6 {
		if err := c.get(ctx, "/x", nil, &out); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 90*time.Millisecond {
		t.Errorf("six calls on an account's key took %v, want them paced at 50/s", elapsed)
	}
}

// Each key spends from its own bucket, kept by the key's hash; the sweep, at
// most once a minute, drops the buckets that have refilled and keeps the rest.
func TestKeyLimiters(t *testing.T) {
	k := newKeyLimiters(0.01, 2) // a token every 100 s
	t0 := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	for i, want := range []time.Duration{0, 0, 100 * time.Second} {
		if got := k.reserve("a", t0); got != want {
			t.Errorf("a's reserve %d = %v, want %v", i, got, want)
		}
	}
	if got := k.reserve("b", t0); got != 0 {
		t.Errorf("b's first reserve = %v, want 0 from its own bucket", got)
	}

	k.reserve("c", t0.Add(30*time.Second))
	if len(k.byKey) != 3 {
		t.Fatalf("limiters = %d before a minute has passed, want 3", len(k.byKey))
	}
	k.reserve("d", t0.Add(101*time.Second))
	_, aKept := k.byKey[keyID("a")]
	_, bKept := k.byKey[keyID("b")]
	if !aKept || bKept || len(k.byKey) != 3 {
		t.Errorf("after the sweep: %d limiters, a kept %t, b kept %t; want a and c still draining, b (refilled) gone, and d", len(k.byKey), aKept, bKept)
	}
}

// A caller that waited on another's page fetch and got that caller's key
// problem fetches again with its own; the one that started it keeps its
// error, and any other failure is shared as before.
func TestPageCacheRetriesAnothersKeyProblem(t *testing.T) {
	c := newPageCache(time.Minute, 10)
	release := make(chan struct{})
	var calls atomic.Int32
	fetch := func(ctx context.Context) (CatalogPage, bool, error) {
		if calls.Add(1) == 1 {
			<-release
			return CatalogPage{}, false, ErrKeyRejected
		}
		return CatalogPage{Metas: []Meta{{ID: "tt1"}}}, true, nil
	}

	started := make(chan error, 1)
	go func() {
		_, err := metasOf(c.load(t.Context(), "k", fetch))
		started <- err
	}()
	for calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	waited := make(chan []Meta, 1)
	go func() {
		metas, err := metasOf(c.load(t.Context(), "k", fetch))
		if err != nil {
			t.Errorf("the waiter: %v, want its own fetch's page", err)
		}
		waited <- metas
	}()
	time.Sleep(10 * time.Millisecond) // let the waiter reach the flight
	close(release)

	if err := <-started; !errors.Is(err, ErrKeyRejected) {
		t.Errorf("the starter: %v, want its key problem", err)
	}
	if metas := <-waited; len(metas) != 1 {
		t.Errorf("the waiter got %v, want the page", metas)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("fetches = %d, want 2", got)
	}
}

// A caller whose retry lands on yet another caller's fetch, which fails on
// that caller's key too, keeps going until it fetches with its own key.
func TestPageCacheWaitsOutEveryOtherKeyProblem(t *testing.T) {
	c := newPageCache(time.Minute, 10)
	first, second := &pageFetch{done: make(chan struct{})}, &pageFetch{done: make(chan struct{})}
	c.inFlight["k"] = first // another caller's fetch, as lookupOrStart files it
	var own atomic.Int32
	fetch := func(context.Context) (CatalogPage, bool, error) {
		own.Add(1)
		return CatalogPage{Metas: []Meta{{ID: "tt1"}}}, true, nil
	}

	got := make(chan []Meta, 1)
	go func() {
		metas, err := metasOf(c.load(t.Context(), "k", fetch))
		if err != nil {
			t.Errorf("load: %v, want its own fetch's page", err)
		}
		got <- metas
	}()
	settle := func(f *pageFetch, next *pageFetch, err error) {
		time.Sleep(20 * time.Millisecond) // let the caller reach f
		c.mu.Lock()
		delete(c.inFlight, "k")
		if next != nil {
			c.inFlight["k"] = next // a third caller, with no key either, started the next fetch
		}
		f.err = err
		c.mu.Unlock()
		close(f.done)
	}
	settle(first, second, ErrNoKey)
	settle(second, nil, ErrKeyRejected)

	if metas := <-got; len(metas) != 1 || metas[0].ID != "tt1" {
		t.Errorf("got %v, want the page", metas)
	}
	if own.Load() != 1 {
		t.Errorf("own fetches = %d, want 1", own.Load())
	}
}
