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

// TestEntityLookupsAreMemoizedPerID covers the per-id memos behind Company,
// Keyword and Collection: a known id reaches TMDB once across repeated calls, and a 404
// comes back as ErrNotFound without becoming a cache entry, so a later call
// asks TMDB again.
func TestEntityLookupsAreMemoizedPerID(t *testing.T) {
	c, hits := fakeEntityTMDB(t)
	ctx := t.Context()

	for range 2 {
		company, err := c.Company(ctx, 1)
		if err != nil {
			t.Fatalf("Company(1): %v", err)
		}
		if company != (Company{ID: 1, Name: "Lucasfilm Ltd."}) {
			t.Fatalf("Company(1) = %+v", company)
		}
		keyword, err := c.Keyword(ctx, 1)
		if err != nil {
			t.Fatalf("Keyword(1): %v", err)
		}
		if keyword != (Keyword{ID: 1, Name: "superhero"}) {
			t.Fatalf("Keyword(1) = %+v", keyword)
		}
		collection, err := c.Collection(ctx, 1)
		if err != nil {
			t.Fatalf("Collection(1): %v", err)
		}
		if collection != (Collection{ID: 1, Name: "Star Wars Collection"}) {
			t.Fatalf("Collection(1) = %+v", collection)
		}
	}
	if got := hits("/company/1"); got != 1 {
		t.Fatalf("/company/1 hit %d times, want 1", got)
	}
	if got := hits("/keyword/1"); got != 1 {
		t.Fatalf("/keyword/1 hit %d times, want 1", got)
	}
	if got := hits("/collection/1"); got != 1 {
		t.Fatalf("/collection/1 hit %d times, want 1", got)
	}

	for range 2 {
		if _, err := c.Company(ctx, 999); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Company(999) error = %v, want ErrNotFound", err)
		}
		if _, err := c.Keyword(ctx, 999); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Keyword(999) error = %v, want ErrNotFound", err)
		}
		if _, err := c.Collection(ctx, 999); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Collection(999) error = %v, want ErrNotFound", err)
		}
	}
	if got := hits("/company/999"); got != 2 {
		t.Fatalf("/company/999 hit %d times, want 2 (a 404 must not be cached)", got)
	}
	if got := hits("/keyword/999"); got != 2 {
		t.Fatalf("/keyword/999 hit %d times, want 2 (a 404 must not be cached)", got)
	}
	if got := hits("/collection/999"); got != 2 {
		t.Fatalf("/collection/999 hit %d times, want 2 (a 404 must not be cached)", got)
	}
	if n := len(c.companies.entries); n != 1 {
		t.Fatalf("company cache holds %d entries, want 1", n)
	}
	if n := len(c.keywords.entries); n != 1 {
		t.Fatalf("keyword cache holds %d entries, want 1", n)
	}
	if n := len(c.collections.entries); n != 1 {
		t.Fatalf("collection cache holds %d entries, want 1", n)
	}
}

// TestWatchProvidersRejectsOddRegion covers the bound on the one memo whose
// key carries client input: a region that isn't shaped like an ISO code
// becomes neither a cache entry nor a TMDB request.
func TestWatchProvidersRejectsOddRegion(t *testing.T) {
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

	_, err := c.WatchProviders(t.Context(), "movie", "not-a-region")
	if !errors.Is(err, ErrInvalidParams) {
		t.Fatalf("WatchProviders error = %v, want ErrInvalidParams", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if hits != 0 {
		t.Fatalf("TMDB hit %d times, want 0 (an unshaped region must not be fetched)", hits)
	}
	if n := len(c.watchProviders.entries); n != 0 {
		t.Fatalf("cache holds %d entries, want 0", n)
	}
}

// TestBoundedMemoHoldsItsCap covers a bounded memo's ceiling: a new key at
// capacity, with nothing expired, empties the memo before it is stored, while
// refreshing a key already held never does.
func TestBoundedMemoHoldsItsCap(t *testing.T) {
	m := newBoundedMemo(0, 3, func(v int) int { return v })
	load := func(key string) {
		t.Helper()
		if _, err := m.load(key, func() (int, error) { return 1, nil }); err != nil {
			t.Fatalf("load(%q): %v", key, err)
		}
	}

	for _, key := range []string{"a", "b", "c"} {
		load(key)
	}
	m.entries["c"] = memoEntry[int]{value: 1, expires: time.Now().Add(-time.Second)}
	load("c")
	if n := len(m.entries); n != 3 {
		t.Fatalf("refreshing a held key left %d entries, want 3", n)
	}

	load("d")
	if n := len(m.entries); n != 1 {
		t.Fatalf("a new key at capacity left %d entries, want 1", n)
	}
	if _, ok := m.entries["d"]; !ok {
		t.Fatal("the new key was not stored")
	}
}

// TestBoundedMemoDropsExpiredFirst covers the cheaper half of making room:
// when entries have expired, only they go, and the live ones stay cached.
func TestBoundedMemoDropsExpiredFirst(t *testing.T) {
	m := newBoundedMemo(time.Hour, 3, func(v int) int { return v })
	for _, key := range []string{"a", "b", "c"} {
		if _, err := m.load(key, func() (int, error) { return 1, nil }); err != nil {
			t.Fatalf("load(%q): %v", key, err)
		}
	}
	m.entries["b"] = memoEntry[int]{value: 1, expires: time.Now().Add(-time.Second)}

	if _, err := m.load("d", func() (int, error) { return 1, nil }); err != nil {
		t.Fatalf("load(d): %v", err)
	}
	for _, key := range []string{"a", "c", "d"} {
		if _, ok := m.entries[key]; !ok {
			t.Fatalf("entry %q was dropped; only the expired one should be", key)
		}
	}
	if _, ok := m.entries["b"]; ok {
		t.Fatal("expired entry b survived")
	}
}

// TestIDKeyedMemosAreBounded pins which memos NewTMDBClient bounds: every
// one keyed by a caller-supplied id, and none of the whole-list memos.
func TestIDKeyedMemosAreBounded(t *testing.T) {
	c := NewTMDBClient("key")
	for name, got := range map[string]int{
		"companies":       c.companies.maxEntries,
		"keywords":        c.keywords.maxEntries,
		"collections":     c.collections.maxEntries,
		"networks":        c.networks.maxEntries,
		"collectionFilms": c.collectionFilms.maxEntries,
		"titleCounts":     c.titleCounts.maxEntries,
	} {
		if got <= 0 {
			t.Fatalf("%s is unbounded", name)
		}
	}
	for name, got := range map[string]int{
		"genres":         c.genres.maxEntries,
		"languages":      c.languages.maxEntries,
		"countries":      c.countries.maxEntries,
		"watchRegions":   c.watchRegions.maxEntries,
		"watchProviders": c.watchProviders.maxEntries,
		"certifications": c.certifications.maxEntries,
		"networkIDs":     c.networkIDs.maxEntries,
	} {
		if got != 0 {
			t.Fatalf("%s is bounded at %d, want unbounded", name, got)
		}
	}
}
