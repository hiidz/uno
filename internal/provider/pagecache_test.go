package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingCatalogTMDB serves one discover page holding one film, its IMDB id
// and the genre list, and counts the discover calls.
func countingCatalogTMDB(t *testing.T) (*TMDBClient, *atomic.Int32) {
	t.Helper()
	var discovers atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/discover/"):
			discovers.Add(1)
			fmt.Fprint(w, `{"results":[{"id":155,"title":"t","genre_ids":[28,80]}],"total_results":1,"total_pages":1}`)
		case strings.HasSuffix(r.URL.Path, "/external_ids"):
			fmt.Fprint(w, `{"imdb_id":"tt0468569"}`)
		case strings.HasPrefix(r.URL.Path, "/genre/"):
			fmt.Fprint(w, `{"genres":[{"id":28,"name":"Action"},{"id":80,"name":"Crime"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c := NewTMDBClient("key")
	c.baseURL = srv.URL
	return c, &discovers
}

// A catalog page is fetched once per discover request and then served from
// the cache: the same recipe, genre and page cost one discover call, and a
// different page or genre is a request of its own. Each caller gets its own
// copy of the page.
func TestFetchCatalogPageIsCachedByRequest(t *testing.T) {
	c, discovers := countingCatalogTMDB(t)
	ctx := t.Context()
	fetch := func(genre string, page int) []Meta {
		t.Helper()
		metas, err := metasOf(c.FetchCatalogPage(ctx, "movie", `{"sort_by":"popularity.desc"}`, genre, page))
		if err != nil || len(metas) != 1 {
			t.Fatalf("FetchCatalogPage = %v, %v; want one meta", metas, err)
		}
		return metas
	}

	first := fetch("", 1)
	first[0].Genres[0] = "changed by a caller"
	if again := fetch("", 1); again[0].Genres[0] != "Action" || discovers.Load() != 1 {
		t.Fatalf("second fetch: genres %q after %d discover calls; want the cached Action after 1", again[0].Genres, discovers.Load())
	}
	fetch("", 2)
	fetch("Crime", 1)
	if got := discovers.Load(); got != 3 {
		t.Errorf("discover calls = %d, want 3: one per distinct request", got)
	}
}

// The key is the discover path and its sorted query, without the API key.
func TestDiscoverPageKey(t *testing.T) {
	p, err := DecodeParams("series", `{"with_genres":"18","sort_by":"popularity.desc"}`)
	if err != nil {
		t.Fatal(err)
	}
	d := discoverPage{endpoint: "/discover/tv", query: p.DiscoverQuery()}
	d.pick(3, false)
	if got, want := d.key(), "/discover/tv?page=3&sort_by=popularity.desc&with_genres=18"; got != want {
		t.Errorf("key = %q, want %q", got, want)
	}
}

// Callers asking for a key while its fetch is in flight share that fetch. A
// caller whose context ends stops waiting, and the fetch still completes and
// is cached for the others.
func TestPageCacheSharesAFetchInFlight(t *testing.T) {
	c := newPageCache(time.Minute, 10)
	release := make(chan struct{})
	var calls atomic.Int32
	fetch := func(ctx context.Context) (CatalogPage, bool, error) {
		calls.Add(1)
		<-release
		return CatalogPage{Metas: []Meta{{ID: "tt1"}}}, true, ctx.Err()
	}

	gone, leave := context.WithCancel(t.Context())
	leftEarly := make(chan error, 1)
	go func() {
		_, err := metasOf(c.load(gone, "k", fetch))
		leftEarly <- err
	}()

	var wg sync.WaitGroup
	results := make([][]Meta, 5)
	for i := range results {
		wg.Go(func() {
			metas, err := metasOf(c.load(t.Context(), "k", fetch))
			if err != nil {
				t.Errorf("load: %v", err)
			}
			results[i] = metas
		})
	}
	leave()
	if err := <-leftEarly; !errors.Is(err, context.Canceled) {
		t.Errorf("the caller that hung up got %v, want context.Canceled", err)
	}
	close(release)
	wg.Wait()

	for i, metas := range results {
		if len(metas) != 1 || metas[0].ID != "tt1" {
			t.Errorf("caller %d got %v, want the shared page", i, metas)
		}
	}
	if metas, err := metasOf(c.load(t.Context(), "k", fetch)); err != nil || len(metas) != 1 {
		t.Errorf("load after the fetch = %v, %v; want the cached page", metas, err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("fetches = %d, want 1", got)
	}
}

// A fetch that fails, panics, or returns a page it says not to keep leaves
// nothing cached, so the next load fetches again; a panic comes back as the
// fetch's error rather than ending the process. The page the second fetch
// keeps is then served from the cache.
func TestPageCacheKeepsOnlyAKeptSuccess(t *testing.T) {
	for name, first := range map[string]pageFetcher{
		"failure": func(context.Context) (CatalogPage, bool, error) { return CatalogPage{}, false, errors.New("TMDB down") },
		"panic":   func(context.Context) (CatalogPage, bool, error) { panic("boom") },
		"not kept": func(context.Context) (CatalogPage, bool, error) {
			return CatalogPage{Metas: []Meta{{ID: "tt0"}}}, false, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := newPageCache(time.Minute, 10)
			var calls atomic.Int32
			fetch := func(ctx context.Context) (CatalogPage, bool, error) {
				if calls.Add(1) == 1 {
					return first(ctx)
				}
				return CatalogPage{Metas: []Meta{{ID: "tt1"}}}, true, nil
			}
			firstMetas, firstErr := metasOf(c.load(t.Context(), "k", fetch))
			if name == "not kept" && (firstErr != nil || len(firstMetas) != 1) {
				t.Fatalf("first load = %v, %v; want the page, served but not kept", firstMetas, firstErr)
			}
			if name != "not kept" && firstErr == nil {
				t.Fatal("first load succeeded, want the fetch's error")
			}
			for range 2 {
				if metas, err := metasOf(c.load(t.Context(), "k", fetch)); err != nil || len(metas) != 1 || metas[0].ID != "tt1" {
					t.Fatalf("later load = %v, %v; want the kept page", metas, err)
				}
			}
			if got := calls.Load(); got != 2 {
				t.Errorf("fetches = %d, want 2", got)
			}
		})
	}
}

// A discover page served without genres, because the genre list failed to
// load, is not cached: the next request fetches it again.
func TestFetchCatalogPageDoesNotCacheAPageWithoutGenres(t *testing.T) {
	var discovers atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/discover/"):
			discovers.Add(1)
			fmt.Fprint(w, `{"results":[{"id":155,"title":"t","genre_ids":[28]}],"total_results":1,"total_pages":1}`)
		case strings.HasSuffix(r.URL.Path, "/external_ids"):
			fmt.Fprint(w, `{"imdb_id":"tt0468569"}`)
		default:
			http.Error(w, "down", http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(srv.Close)
	c := NewTMDBClient("key")
	c.baseURL = srv.URL

	for range 2 {
		if metas, err := metasOf(c.FetchCatalogPage(t.Context(), "movie", `{}`, "", 1)); err != nil || len(metas) != 1 || metas[0].Genres != nil {
			t.Fatalf("FetchCatalogPage = %v, %v; want one meta without genres", metas, err)
		}
	}
	if got := discovers.Load(); got != 2 {
		t.Errorf("discover calls = %d, want 2: a page without genres isn't cached", got)
	}
}

// An entry is served until its TTL passes, then dropped; past max entries
// the least recently used goes first.
func TestPageCacheExpiresAndEvicts(t *testing.T) {
	c := newPageCache(time.Minute, 2)
	t0 := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	page := CatalogPage{Metas: []Meta{{ID: "tt1"}}}

	c.store("a", page, t0)
	if _, ok := c.fresh("a", t0.Add(time.Minute)); !ok {
		t.Error("an entry at its TTL is gone, want it served")
	}
	if _, ok := c.fresh("a", t0.Add(time.Minute+time.Second)); ok {
		t.Error("an entry past its TTL is served")
	}
	if _, present := c.entries["a"]; present || c.order.Len() != 0 {
		t.Error("an expired entry is still held")
	}

	c.store("a", page, t0)
	c.store("b", page, t0)
	c.fresh("a", t0)
	c.store("c", page, t0)
	for key, want := range map[string]bool{"a": true, "b": false, "c": true} {
		if _, ok := c.fresh(key, t0); ok != want {
			t.Errorf("%s cached = %v, want %v", key, ok, want)
		}
	}
}
