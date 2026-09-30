package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
)

// fakeDiscover serves a discover endpoint with totalPages pages, one result per
// page whose id is the page number, and records every page requested.
func fakeDiscover(t *testing.T, totalPages int) (*TMDBClient, func() []int) {
	t.Helper()
	var mu sync.Mutex
	var pages []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		mu.Lock()
		pages = append(pages, page)
		mu.Unlock()
		results := "[]"
		if page >= 1 && page <= totalPages {
			results = fmt.Sprintf(`[{"id":%d,"title":"t"}]`, page)
		}
		fmt.Fprintf(w, `{"results":%s,"total_results":%d,"total_pages":%d}`, results, totalPages, totalPages)
	}))
	t.Cleanup(srv.Close)

	c := NewTMDBClient("key")
	c.baseURL = srv.URL
	// The shuffle tests send hundreds of requests; pacing them isn't what
	// they test.
	c.limiter = newLimiter(1e9, 1e9)
	return c, func() []int {
		mu.Lock()
		defer mu.Unlock()
		return append([]int(nil), pages...)
	}
}

func TestPreviewCatalogPlainRecipeIsPageOne(t *testing.T) {
	c, requested := fakeDiscover(t, 50)
	items, _, randomized, err := c.PreviewCatalog(context.Background(), "movie", `{}`, "")
	if err != nil {
		t.Fatal(err)
	}
	if randomized || len(items) != 1 || items[0].TMDBID != 1 {
		t.Fatalf("got randomized=%v items=%v, want page 1 unshuffled", randomized, items)
	}
	if got := requested(); len(got) != 1 || got[0] != 1 {
		t.Fatalf("requested pages %v, want [1]", got)
	}
}

func TestPreviewCatalogShufflesWithinExistingPages(t *testing.T) {
	c, _ := fakeDiscover(t, 3)
	seen := map[int]bool{}
	for range 100 {
		items, _, randomized, err := c.PreviewCatalog(context.Background(), "movie", `{"randomized":true}`, "")
		if err != nil {
			t.Fatal(err)
		}
		if !randomized || len(items) != 1 {
			t.Fatalf("got randomized=%v items=%v, want one shuffled result", randomized, items)
		}
		if items[0].TMDBID > 3 {
			t.Fatalf("got page %d, past the last page", items[0].TMDBID)
		}
		seen[items[0].TMDBID] = true
	}
	if len(seen) < 2 {
		t.Fatalf("100 shuffled runs only ever returned pages %v", seen)
	}
}

func TestPreviewCatalogShuffleCapsAtMaxRandomPage(t *testing.T) {
	c, requested := fakeDiscover(t, 500)
	for range 100 {
		if _, _, _, err := c.PreviewCatalog(context.Background(), "movie", `{"randomized":true}`, ""); err != nil {
			t.Fatal(err)
		}
	}
	for _, page := range requested() {
		if page < 1 || page > maxRandomPage {
			t.Fatalf("requested page %d, outside 1..%d", page, maxRandomPage)
		}
	}
}

// A genre narrows the preview exactly as a client's pick narrows the addon
// path: resolved by name through GenreExtraOptions, then ANDed onto the
// recipe's with_genres. A name outside those options leaves it unfiltered.
func TestPreviewCatalogAppliesGenre(t *testing.T) {
	var mu sync.Mutex
	var withGenres []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/genre/movie/list" {
			fmt.Fprint(w, `{"genres":[{"id":37,"name":"Western"},{"id":10752,"name":"War"}]}`)
			return
		}
		mu.Lock()
		withGenres = append(withGenres, r.URL.Query().Get("with_genres"))
		mu.Unlock()
		fmt.Fprint(w, `{"results":[],"total_results":0,"total_pages":1}`)
	}))
	t.Cleanup(srv.Close)
	c := NewTMDBClient("key")
	c.baseURL = srv.URL

	for _, genre := range []string{"Western", "", "Not a genre"} {
		if _, _, _, err := c.PreviewCatalog(context.Background(), "movie", `{"with_genres":"10752"}`, genre); err != nil {
			t.Fatalf("genre %q: %v", genre, err)
		}
	}

	// War is already required by the recipe, so it is not an option; Western is.
	want := []string{"10752,37", "10752", "10752"}
	if fmt.Sprint(withGenres) != fmt.Sprint(want) {
		t.Fatalf("with_genres sent = %q, want %q", withGenres, want)
	}
}
