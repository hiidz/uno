package provider

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestMemoCachesUntilTTL covers both lifetimes a memo is built with: a zero
// ttl serves the first fetch forever, a positive one stops serving it once
// it passes.
func TestMemoCachesUntilTTL(t *testing.T) {
	t.Run("zero ttl fetches once", func(t *testing.T) {
		var fetches int
		m := newMemo(0, func(v []int) []int { return v })

		for range 3 {
			if _, err := m.load("k", func() ([]int, error) {
				fetches++
				return []int{1}, nil
			}); err != nil {
				t.Fatalf("load: %v", err)
			}
		}
		if fetches != 1 {
			t.Fatalf("fetches = %d, want 1", fetches)
		}
	})

	t.Run("expired entry refetches", func(t *testing.T) {
		var fetches int
		m := newMemo(time.Nanosecond, func(v []int) []int { return v })
		fetch := func() ([]int, error) {
			fetches++
			return []int{1}, nil
		}

		if _, err := m.load("k", fetch); err != nil {
			t.Fatalf("load: %v", err)
		}
		time.Sleep(time.Millisecond)
		if _, err := m.load("k", fetch); err != nil {
			t.Fatalf("load: %v", err)
		}
		if fetches != 2 {
			t.Fatalf("fetches = %d, want 2", fetches)
		}
	})
}

// TestMemoKeysAreIndependent guards the one thing a shared cache must not
// do: answer for a key it was never asked about. WatchProviders keys by
// catalog type and region, so a leak here would serve one market's
// provider ids for another.
func TestMemoKeysAreIndependent(t *testing.T) {
	m := newMemo(0, func(v string) string { return v })

	for _, key := range []string{"movie/US", "movie/GB"} {
		got, err := m.load(key, func() (string, error) { return key, nil })
		if err != nil {
			t.Fatalf("load(%q): %v", key, err)
		}
		if got != key {
			t.Fatalf("load(%q) = %q, want %q", key, got, key)
		}
	}
}

// TestMemoDoesNotCacheFailures proves a TMDB outage doesn't poison the
// cache for the process's lifetime — the next call has to retry.
func TestMemoDoesNotCacheFailures(t *testing.T) {
	m := newMemo(0, func(v []int) []int { return v })
	wantErr := errors.New("tmdb down")

	if _, err := m.load("k", func() ([]int, error) { return nil, wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("load error = %v, want %v", err, wantErr)
	}

	got, err := m.load("k", func() ([]int, error) { return []int{7}, nil })
	if err != nil {
		t.Fatalf("load after failure: %v", err)
	}
	if len(got) != 1 || got[0] != 7 {
		t.Fatalf("load after failure = %v, want [7]", got)
	}
}

// TestMemoClonesOnRead is the property the whole type rests on: callers get
// their own copy, so one of them sorting or overwriting what it got back
// can't reach the cached value. WatchProviders sorts in place, which is
// exactly this hazard.
func TestMemoClonesOnRead(t *testing.T) {
	m := newMemo(0, func(v []int) []int { return append([]int(nil), v...) })
	fetch := func() ([]int, error) { return []int{1, 2, 3}, nil }

	first, err := m.load("k", fetch)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	first[0] = 99

	second, err := m.load("k", fetch)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if second[0] != 1 {
		t.Fatalf("caller mutation reached the cache: got %v", second)
	}
}

// TestCloneCertificationsIsDeep covers the one memo value that isn't a
// plain slice: maps.Clone would leave every country's scale shared with
// the cache.
func TestCloneCertificationsIsDeep(t *testing.T) {
	original := map[string][]Certification{"US": {{Certification: "PG"}, {Certification: "R"}}}

	clone := cloneCertifications(original)
	clone["US"][0].Certification = "MUTATED"

	if original["US"][0].Certification != "PG" {
		t.Fatalf("clone shares its backing array with the original: got %q", original["US"][0].Certification)
	}
}

// TestMemoConcurrentLoad is here for the race detector: concurrent readers
// and a writer over the same key must not trip it.
func TestMemoConcurrentLoad(t *testing.T) {
	m := newMemo(0, func(v []int) []int { return append([]int(nil), v...) })

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			got, err := m.load("k", func() ([]int, error) { return []int{1, 2, 3}, nil })
			if err != nil {
				t.Errorf("load: %v", err)
				return
			}
			got[0] = 99 // each caller owns its copy
		})
	}
	wg.Wait()
}

// TestClientListsAreMemoized exercises the wiring in NewTMDBClient rather
// than a memo built in the test: each list method must hit TMDB once and
// then answer from its own memo, and the value it hands back must be a
// copy, so a caller mutating it can't reach what the next caller gets.
func TestClientListsAreMemoized(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()

		switch r.URL.Path {
		case "/genre/movie/list":
			fmt.Fprint(w, `{"genres":[{"id":28,"name":"Action"}]}`)
		case "/configuration/languages":
			fmt.Fprint(w, `[{"iso_639_1":"en","english_name":"English"}]`)
		case "/configuration/countries":
			fmt.Fprint(w, `[{"iso_3166_1":"US","english_name":"United States"}]`)
		case "/watch/providers/regions":
			fmt.Fprint(w, `{"results":[{"iso_3166_1":"US","english_name":"United States"}]}`)
		case "/watch/providers/movie":
			fmt.Fprint(w, `{"results":[{"provider_id":8,"provider_name":"Netflix","display_priority":1}]}`)
		case "/certification/movie/list":
			fmt.Fprint(w, `{"certifications":{"US":[{"certification":"PG"}]}}`)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewTMDBClient("key")
	c.baseURL = srv.URL
	ctx := t.Context()

	// Each entry calls one list method twice and reports the first result's
	// length, so the subtest can mutate that result and prove the second
	// call is unaffected by it.
	lists := []struct {
		name, path string
		call       func() (int, error)
	}{
		{"genres", "/genre/movie/list", func() (int, error) {
			first, err := c.Genres(ctx, "movie")
			if err != nil {
				return 0, err
			}
			clear(first)
			second, err := c.Genres(ctx, "movie")
			if err != nil {
				return 0, err
			}
			if second[0].Name != "Action" {
				return 0, fmt.Errorf("cached genre was mutated by a caller: %+v", second[0])
			}
			return len(second), nil
		}},
		{"languages", "/configuration/languages", func() (int, error) {
			if _, err := c.Languages(ctx); err != nil {
				return 0, err
			}
			second, err := c.Languages(ctx)
			return len(second), err
		}},
		{"countries", "/configuration/countries", func() (int, error) {
			if _, err := c.Countries(ctx); err != nil {
				return 0, err
			}
			second, err := c.Countries(ctx)
			return len(second), err
		}},
		{"watch regions", "/watch/providers/regions", func() (int, error) {
			if _, err := c.WatchRegions(ctx); err != nil {
				return 0, err
			}
			second, err := c.WatchRegions(ctx)
			return len(second), err
		}},
		{"watch providers", "/watch/providers/movie", func() (int, error) {
			if _, err := c.WatchProviders(ctx, "movie", "US"); err != nil {
				return 0, err
			}
			second, err := c.WatchProviders(ctx, "movie", "US")
			return len(second), err
		}},
		{"certifications", "/certification/movie/list", func() (int, error) {
			first, err := c.Certifications(ctx, "movie")
			if err != nil {
				return 0, err
			}
			clear(first["US"])
			second, err := c.Certifications(ctx, "movie")
			if err != nil {
				return 0, err
			}
			if second["US"][0].Certification != "PG" {
				return 0, fmt.Errorf("cached certification scale was mutated by a caller: %+v", second["US"][0])
			}
			return len(second), nil
		}},
	}

	for _, list := range lists {
		t.Run(list.name, func(t *testing.T) {
			n, err := list.call()
			if err != nil {
				t.Fatalf("%s: %v", list.name, err)
			}
			if n != 1 {
				t.Fatalf("%s: second call returned %d entries, want 1", list.name, n)
			}

			mu.Lock()
			got := hits[list.path]
			mu.Unlock()
			if got != 1 {
				t.Fatalf("%s: TMDB hit %d times, want 1", list.name, got)
			}
		})
	}
}

// TestWatchProvidersSkipsCacheForOddRegion covers the bound on the one memo
// whose key carries client input: a region that isn't shaped like an ISO
// code must not become a cache entry.
func TestWatchProvidersSkipsCacheForOddRegion(t *testing.T) {
	var hits int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		fmt.Fprint(w, `{"results":[]}`)
	}))
	t.Cleanup(srv.Close)

	c := NewTMDBClient("key")
	c.baseURL = srv.URL

	for range 2 {
		if _, err := c.WatchProviders(t.Context(), "movie", "not-a-region"); err != nil {
			t.Fatalf("WatchProviders: %v", err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if hits != 2 {
		t.Fatalf("TMDB hit %d times, want 2 (an unshaped region must not be cached)", hits)
	}
	if n := len(c.watchProviders.entries); n != 0 {
		t.Fatalf("cache holds %d entries, want 0", n)
	}
}
