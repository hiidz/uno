package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeCatalogTMDB serves one discover page with a single item, its
// external_ids, and the genre list. genreListStatus lets a test fail the
// genre-list fetch.
func fakeCatalogTMDB(t *testing.T, item string, genreListStatus int) *TMDBClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/discover/"):
			fmt.Fprintf(w, `{"results":[%s],"total_results":1,"total_pages":1}`, item)
		case strings.HasSuffix(r.URL.Path, "/external_ids"):
			fmt.Fprint(w, `{"imdb_id":"tt0468569"}`)
		case strings.HasPrefix(r.URL.Path, "/genre/"):
			if genreListStatus != http.StatusOK {
				w.WriteHeader(genreListStatus)
				return
			}
			fmt.Fprint(w, `{"genres":[{"id":28,"name":"Action"},{"id":80,"name":"Crime"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewTMDBClient("key")
	c.baseURL = srv.URL
	return c
}

func TestFetchCatalogPageMapsDiscoverFields(t *testing.T) {
	c := fakeCatalogTMDB(t, `{"id":155,"title":"The Dark Knight","overview":"o",
		"poster_path":"/p.jpg","backdrop_path":"/b.jpg","genre_ids":[80,28,9999],
		"release_date":"2008-07-16"}`, http.StatusOK)

	metas, err := c.FetchCatalogPage(context.Background(), "movie", `{}`, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []Meta{{
		ID:          "tt0468569",
		Type:        "movie",
		Name:        "The Dark Knight",
		Poster:      "https://image.tmdb.org/t/p/w500/p.jpg",
		Background:  "https://image.tmdb.org/t/p/w1280/b.jpg",
		Description: "o",
		ReleaseInfo: "2008",
		Released:    "2008-07-16T00:00:00.000Z",
		Genres:      []string{"Crime", "Action"},
	}}
	if !reflect.DeepEqual(metas, want) {
		t.Fatalf("got %+v\nwant %+v", metas, want)
	}
}

func TestFetchCatalogPageServesWithoutGenresWhenGenreListFails(t *testing.T) {
	c := fakeCatalogTMDB(t, `{"id":155,"title":"t","genre_ids":[28],"release_date":""}`,
		http.StatusInternalServerError)

	metas, err := c.FetchCatalogPage(context.Background(), "movie", `{}`, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].Genres != nil || metas[0].Background != "" || metas[0].Released != "" {
		t.Fatalf("got %+v, want one meta with no genres, background, or released", metas)
	}
}

// TestFetchCatalogPageSendsEntityFilters proves with_companies and
// with_keywords reach the discover request TMDB sees, on both discover
// endpoints, with their AND/OR separators intact.
func TestFetchCatalogPageSendsEntityFilters(t *testing.T) {
	for _, tc := range []struct{ catalogType, path string }{
		{"movie", "/discover/movie"},
		{"series", "/discover/tv"},
	} {
		t.Run(tc.catalogType, func(t *testing.T) {
			var mu sync.Mutex
			var seen url.Values
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					http.NotFound(w, r)
					return
				}
				mu.Lock()
				seen = r.URL.Query()
				mu.Unlock()
				fmt.Fprint(w, `{"results":[],"total_results":0,"total_pages":1}`)
			}))
			t.Cleanup(srv.Close)
			c := NewTMDBClient("key")
			c.baseURL = srv.URL

			params := `{"with_companies":"420|2","with_keywords":"9715,180547"}`
			if _, err := c.FetchCatalogPage(t.Context(), tc.catalogType, params, "", 1); err != nil {
				t.Fatal(err)
			}

			mu.Lock()
			defer mu.Unlock()
			if got := seen.Get("with_companies"); got != "420|2" {
				t.Fatalf("with_companies = %q, want %q", got, "420|2")
			}
			if got := seen.Get("with_keywords"); got != "9715,180547" {
				t.Fatalf("with_keywords = %q, want %q", got, "9715,180547")
			}
		})
	}
}

// fakeRandomizedTMDB serves a discover endpoint holding totalPages pages,
// empty past the last one, and records every page requested.
// externalIDsStatus lets a test fail the per-item external_ids lookup.
func fakeRandomizedTMDB(t *testing.T, totalPages, externalIDsStatus int) (*TMDBClient, func() []int) {
	t.Helper()
	var mu sync.Mutex
	var pages []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/discover/"):
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			mu.Lock()
			pages = append(pages, page)
			mu.Unlock()

			results := "[]"
			if page >= 1 && page <= totalPages {
				results = fmt.Sprintf(`[{"id":%d,"title":"t"}]`, page)
			}
			fmt.Fprintf(w, `{"results":%s,"total_results":1,"total_pages":%d}`, results, totalPages)
		case strings.HasSuffix(r.URL.Path, "/external_ids"):
			if externalIDsStatus != http.StatusOK {
				w.WriteHeader(externalIDsStatus)
				return
			}
			fmt.Fprint(w, `{"imdb_id":"tt0468569"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewTMDBClient("key")
	c.baseURL = srv.URL
	return c, func() []int {
		mu.Lock()
		defer mu.Unlock()
		return append([]int(nil), pages...)
	}
}

// TestFetchCatalogPageRandomizedFallsBackToPageOne covers a randomized
// recipe narrow enough that most of the [1, maxRandomPage] range is past
// its last page: the row must still have tiles on it.
func TestFetchCatalogPageRandomizedFallsBackToPageOne(t *testing.T) {
	c, requested := fakeRandomizedTMDB(t, 1, http.StatusOK)

	for range 20 {
		metas, err := c.FetchCatalogPage(context.Background(), "movie", `{"randomized":true}`, "", 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(metas) != 1 {
			t.Fatalf("got %d metas, want 1", len(metas))
		}
	}

	// Every overshooting pick costs a second call, and only page 1 can
	// answer; a pick that lands on page 1 costs one.
	for _, page := range requested() {
		if page < 1 || page > maxRandomPage {
			t.Fatalf("requested page %d, want it within [1, %d]", page, maxRandomPage)
		}
	}
}

// TestFetchCatalogPageRandomizedKeepsAPopulatedPick covers the common case
// the fallback must not cost anything: a pick TMDB has results for is
// served from that one call.
func TestFetchCatalogPageRandomizedKeepsAPopulatedPick(t *testing.T) {
	c, requested := fakeRandomizedTMDB(t, maxRandomPage, http.StatusOK)

	if _, err := c.FetchCatalogPage(context.Background(), "movie", `{"randomized":true}`, "", 1); err != nil {
		t.Fatal(err)
	}
	if pages := requested(); len(pages) != 1 {
		t.Fatalf("requested pages %v, want exactly one discover call", pages)
	}
}

// TestFetchCatalogPageFailsWhenExternalIDsFail covers the difference
// between "TMDB has no IMDB id for this title" and "the lookup failed": the
// second must not serve a short page the client caches for hours.
func TestFetchCatalogPageFailsWhenExternalIDsFail(t *testing.T) {
	c, _ := fakeRandomizedTMDB(t, 1, http.StatusInternalServerError)

	metas, err := c.FetchCatalogPage(context.Background(), "movie", `{}`, "", 1)
	if err == nil {
		t.Fatalf("got %d metas and no error, want an error", len(metas))
	}
}

// TestFetchCatalogPageDropsItemsTMDBHasNoExternalIDsFor covers the other
// half of that split: a 404 on external_ids is as permanent as an empty
// imdb_id, so the item is dropped and the rest of the page is served.
func TestFetchCatalogPageDropsItemsTMDBHasNoExternalIDsFor(t *testing.T) {
	c, _ := fakeRandomizedTMDB(t, 1, http.StatusNotFound)

	metas, err := c.FetchCatalogPage(context.Background(), "movie", `{}`, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 0 {
		t.Fatalf("got %+v, want the item dropped", metas)
	}
}
