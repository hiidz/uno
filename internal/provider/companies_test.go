package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync"
	"testing"
)

// companyTitleCounts is what the fake's discover credits to each company on
// its search page, in TMDB's order. 11 and 12 fall past maxCompanyMatches.
var companyTitleCounts = []int{0, 1, 176, 9, 5, 4, 9, 50, 0, 12, 999, 999}

// fakeCompanySearchTMDB serves a /search/company page listing companies 1–12
// (only 3 has an origin country) and a /discover/{movie,tv} answering
// with_companies=<id> with companyTitleCounts[id-1] as total_results — or with
// failStatus for company failID. hits counts discover requests per
// "path?with_companies" key; total counts every request.
func fakeCompanySearchTMDB(t *testing.T, failID, failStatus int) (c *TMDBClient, hits func(key string) int, total func() int) {
	t.Helper()
	var mu sync.Mutex
	seen := map[string]int{}
	var all int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		all++
		mu.Unlock()
		switch r.URL.Path {
		case "/search/company":
			fmt.Fprint(w, `{"page":1,"results":[`)
			for id := 1; id <= len(companyTitleCounts); id++ {
				if id > 1 {
					fmt.Fprint(w, ",")
				}
				country := ""
				if id == 3 {
					country = "US"
				}
				fmt.Fprintf(w, `{"id":%d,"name":"A24","origin_country":%q}`, id, country)
			}
			fmt.Fprint(w, `],"total_pages":1,"total_results":12}`)
		case "/discover/movie", "/discover/tv":
			id, _ := strconv.Atoi(r.URL.Query().Get("with_companies"))
			mu.Lock()
			seen[r.URL.Path+"?"+strconv.Itoa(id)]++
			mu.Unlock()
			if id == failID {
				w.WriteHeader(failStatus)
				return
			}
			fmt.Fprintf(w, `{"page":1,"results":[],"total_pages":1,"total_results":%d}`, companyTitleCounts[id-1])
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c = NewTMDBClient("key")
	c.baseURL = srv.URL
	return c, func(key string) int {
			mu.Lock()
			defer mu.Unlock()
			return seen[key]
		}, func() int {
			mu.Lock()
			defer mu.Unlock()
			return all
		}
}

// TestSearchCompaniesRanksAndFilters covers the whole enrichment: only the
// first ten results are counted, those under minCompanyTitles are dropped
// (five is kept), and the rest are ordered by count with ties in TMDB's order.
func TestSearchCompaniesRanksAndFilters(t *testing.T) {
	c, hits, _ := fakeCompanySearchTMDB(t, 0, 0)

	got, err := c.SearchCompanies(t.Context(), "movie", "a24")
	if err != nil {
		t.Fatalf("SearchCompanies: %v", err)
	}
	want := []CompanyMatch{
		{ID: 3, Name: "A24", OriginCountry: "US", TitleCount: 176},
		{ID: 8, Name: "A24", TitleCount: 50},
		{ID: 10, Name: "A24", TitleCount: 12},
		{ID: 4, Name: "A24", TitleCount: 9},
		{ID: 7, Name: "A24", TitleCount: 9},
		{ID: 5, Name: "A24", TitleCount: 5},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SearchCompanies = %+v, want %+v", got, want)
	}
	body, err := json.Marshal(got[:2])
	if err != nil {
		t.Fatal(err)
	}
	if want := `[{"id":3,"name":"A24","origin_country":"US","title_count":176},{"id":8,"name":"A24","origin_country":"","title_count":50}]`; string(body) != want {
		t.Fatalf("wire shape = %s, want %s", body, want)
	}
	for id := 1; id <= 10; id++ {
		if n := hits("/discover/movie?" + strconv.Itoa(id)); n != 1 {
			t.Fatalf("company %d counted %d times, want 1", id, n)
		}
	}
	for _, id := range []string{"11", "12"} {
		if n := hits("/discover/movie?" + id); n != 0 {
			t.Fatalf("company %s past the first ten counted %d times, want 0", id, n)
		}
	}
}

// TestSearchCompaniesMemoizesCountsPerType covers the count memo: a repeat
// search counts nothing again, and a count is kept per catalog type, since a
// studio's film and series counts differ.
func TestSearchCompaniesMemoizesCountsPerType(t *testing.T) {
	c, hits, _ := fakeCompanySearchTMDB(t, 0, 0)

	for _, catalogType := range []string{"movie", "movie", "series", "series"} {
		if _, err := c.SearchCompanies(t.Context(), catalogType, "a24"); err != nil {
			t.Fatalf("SearchCompanies(%s): %v", catalogType, err)
		}
	}
	for _, path := range []string{"/discover/movie?3", "/discover/tv?3"} {
		if n := hits(path); n != 1 {
			t.Fatalf("%s requested %d times, want 1", path, n)
		}
	}
}

// TestSearchCompaniesRequiresCatalogType covers the type param: missing or
// unknown is rejected before TMDB is contacted.
func TestSearchCompaniesRequiresCatalogType(t *testing.T) {
	c, _, total := fakeCompanySearchTMDB(t, 0, 0)

	for _, catalogType := range []string{"", "tv", "anime"} {
		if _, err := c.SearchCompanies(t.Context(), catalogType, "a24"); !errors.Is(err, ErrInvalidCatalogType) {
			t.Fatalf("SearchCompanies(%q) error = %v, want ErrInvalidCatalogType", catalogType, err)
		}
	}
	if n := total(); n != 0 {
		t.Fatalf("TMDB saw %d requests, want 0", n)
	}
}

// TestSearchCompaniesFailsOnCountFailure covers a count that fails: the
// search fails rather than dropping that result, a 404 does not surface as
// ErrNotFound (the route would answer 404 for a search), and the failure is
// not cached.
func TestSearchCompaniesFailsOnCountFailure(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusNotFound} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			c, hits, _ := fakeCompanySearchTMDB(t, 4, status)

			for range 2 {
				got, err := c.SearchCompanies(t.Context(), "movie", "a24")
				if err == nil {
					t.Fatalf("SearchCompanies = %+v, want an error", got)
				}
				if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidParams) {
					t.Fatalf("SearchCompanies error = %v, want neither ErrNotFound nor ErrInvalidParams", err)
				}
			}
			if n := hits("/discover/movie?4"); n != 2 {
				t.Fatalf("failing count requested %d times, want 2 (a failure must not be cached)", n)
			}
		})
	}
}
