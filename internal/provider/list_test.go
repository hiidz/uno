package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// Every list a type offers validates for it, the other type's own lists
// don't, and shuffle is the one field allowed beside a list.
func TestValidateTMDBList(t *testing.T) {
	for catalogType, lists := range listEndpoints {
		for list := range lists {
			p, err := DecodeParams(catalogType, fmt.Sprintf(`{"tmdb_list":%q,"randomized":true}`, list))
			if err != nil {
				t.Fatalf("%s %s: %v", catalogType, list, err)
			}
			if err := p.Validate(); err != nil {
				t.Errorf("%s list %s = %v, want it accepted", catalogType, list, err)
			}
		}
	}
	for _, tc := range []struct{ catalogType, list string }{
		{"series", "now_playing"},
		{"series", "upcoming"},
		{"movie", "airing_today"},
		{"movie", "on_the_air"},
		{"movie", "trending/month"},
		{"movie", "../person/1"},
	} {
		p := paramsOf(t, tc.catalogType, fmt.Sprintf(`{"tmdb_list":%q}`, tc.list))
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "tmdb_list") {
			t.Errorf("%s list %q = %v, want an error naming tmdb_list", tc.catalogType, tc.list, err)
		}
	}
}

// A list takes no filters: each other field beside it is refused by name.
func TestValidateRefusesFieldsBesideATMDBList(t *testing.T) {
	for _, tc := range []struct{ catalogType, field, params string }{
		{"movie", "sort_by", `"sort_by":"popularity.desc"`},
		{"movie", "with_genres", `"with_genres":"28"`},
		{"movie", "with_collection", `"with_collection":"10"`},
		{"movie", "with_cast", `"with_cast":"1"`},
		{"movie", "released_within_days", `"released_within_days":30`},
		{"series", "with_networks", `"with_networks":"213"`},
		{"series", "with_origin_country", `"with_origin_country":"KR"`},
		{"series", "aired_within_days", `"aired_within_days":7`},
	} {
		p := paramsOf(t, tc.catalogType, `{"tmdb_list":"popular",`+tc.params+`}`)
		err := p.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.field+" cannot be combined with tmdb_list") {
			t.Errorf("%s list with %s = %v, want it refused by name", tc.catalogType, tc.field, err)
		}
	}
}

func paramsOf(t *testing.T, catalogType, params string) CatalogParams {
	t.Helper()
	p, err := DecodeParams(catalogType, params)
	if err != nil {
		t.Fatalf("DecodeParams(%s): %v", params, err)
	}
	return p
}

// fakeListTMDB serves every path with totalPages pages of one item each, the
// item's id being the page, and records each path and query it was asked for
// but the genre list, which every page reads to name its tiles' genres.
func fakeListTMDB(t *testing.T, totalPages int) (*TMDBClient, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/external_ids") {
			fmt.Fprint(w, `{"imdb_id":"tt0000001"}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/genre/") {
			fmt.Fprint(w, `{"genres":[{"id":18,"name":"Drama"}]}`)
			return
		}
		mu.Lock()
		seen = append(seen, r.URL.Path+"?"+strings.ReplaceAll(r.URL.Query().Encode(), "api_key=key&", ""))
		mu.Unlock()
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		results := "[]"
		if page >= 1 && page <= totalPages {
			results = fmt.Sprintf(`[{"id":%d,"title":"t","name":"t"}]`, page)
		}
		fmt.Fprintf(w, `{"results":%s,"total_results":%d,"total_pages":%d}`, results, totalPages, totalPages)
	}))
	t.Cleanup(srv.Close)
	c := NewTMDBClient("key")
	c.baseURL = srv.URL
	c.limiter = newLimiter(1e9, 1e9)
	return c, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

// Each list is served from its own path with the page as its only query, by
// both the addon's page fetch and the builder's preview.
func TestListRecipesReadTheirOwnPath(t *testing.T) {
	for catalogType, lists := range listEndpoints {
		for list, path := range lists {
			c, seen := fakeListTMDB(t, 5)
			params := fmt.Sprintf(`{"tmdb_list":%q}`, list)

			page, err := c.FetchCatalogPage(t.Context(), catalogType, params, "", 2)
			if err != nil {
				t.Fatalf("%s %s: FetchCatalogPage: %v", catalogType, list, err)
			}
			if !page.More || len(page.Metas) != 1 {
				t.Errorf("%s %s page 2 = %+v, want one meta and more pages", catalogType, list, page)
			}
			if _, _, _, err := c.PreviewCatalog(t.Context(), catalogType, params, ""); err != nil {
				t.Fatalf("%s %s: PreviewCatalog: %v", catalogType, list, err)
			}
			want := []string{path + "?page=2", path + "?page=1"}
			if got := seen(); fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("%s %s asked TMDB for %v, want %v", catalogType, list, got, want)
			}
		}
	}
}

// A shuffled list picks a page within the first maxRandomPage and falls back
// to page 1 past the list's end, as a discover recipe does.
func TestShuffledListFallsBackToPageOne(t *testing.T) {
	c, seen := fakeListTMDB(t, 1)
	for range 20 {
		page, err := c.FetchCatalogPage(t.Context(), "movie", `{"tmdb_list":"popular","randomized":true}`, "", 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Metas) != 1 {
			t.Fatalf("shuffled page = %+v, want page 1's meta", page)
		}
	}
	for _, request := range seen() {
		if !strings.HasPrefix(request, "/movie/popular?page=") {
			t.Fatalf("asked TMDB for %s, want /movie/popular only", request)
		}
	}
}

// A list recipe offers no genres and asks TMDB nothing for them; a picked
// genre serves the list as it is.
func TestListRecipeHasNoGenres(t *testing.T) {
	c, seen := fakeListTMDB(t, 1)
	options, err := c.GenreExtraOptions(t.Context(), "series", `{"tmdb_list":"trending/week"}`)
	if err != nil || options == nil || len(options) != 0 {
		t.Fatalf("GenreExtraOptions = %v, %v; want an empty list", options, err)
	}
	if got := seen(); len(got) != 0 {
		t.Fatalf("asked TMDB for %v, want nothing", got)
	}
	if _, err := c.FetchCatalogPage(t.Context(), "series", `{"tmdb_list":"trending/week"}`, "Drama", 1); err != nil {
		t.Fatal(err)
	}
	for _, request := range seen() {
		if request != "/trending/tv/week?page=1" {
			t.Fatalf("a genre pick asked TMDB for %s, want the list unfiltered", request)
		}
	}
}
