package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
)

// fakeSearchTMDB serves /search/company, /search/keyword and
// /search/collection, answering "none" with an empty result page and
// anything else with one hit, plus a /discover/movie crediting 30 titles to
// any company. It records every query TMDB is sent.
func fakeSearchTMDB(t *testing.T) (*TMDBClient, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		mu.Lock()
		queries = append(queries, r.URL.Path+"?"+q)
		mu.Unlock()

		if q == "none" {
			fmt.Fprint(w, `{"page":1,"results":[],"total_pages":1,"total_results":0}`)
			return
		}
		switch r.URL.Path {
		case "/search/company":
			fmt.Fprint(w, `{"page":1,"results":[{"id":420,"name":"Marvel Studios","logo_path":"/l.png","origin_country":"US"}],"total_pages":1,"total_results":1}`)
		case "/search/keyword":
			fmt.Fprint(w, `{"page":1,"results":[{"id":9715,"name":"superhero"}],"total_pages":1,"total_results":1}`)
		case "/discover/movie":
			fmt.Fprint(w, `{"page":1,"results":[],"total_pages":2,"total_results":30}`)
		case "/search/collection":
			fmt.Fprint(w, `{"page":1,"results":[{"id":10,"name":"Star Wars Collection","original_language":"en","poster_path":"/p.jpg","adult":false}],"total_pages":1,"total_results":1}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewTMDBClient("key")
	c.baseURL = srv.URL
	return c, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), queries...)
	}
}

func TestSearchDecodesFirstPage(t *testing.T) {
	c, queries := fakeSearchTMDB(t)
	ctx := t.Context()

	companies, err := c.SearchCompanies(ctx, "movie", " marvel ")
	if err != nil {
		t.Fatalf("SearchCompanies: %v", err)
	}
	if want := []CompanyMatch{{ID: 420, Name: "Marvel Studios", OriginCountry: "US", TitleCount: 30}}; !reflect.DeepEqual(companies, want) {
		t.Fatalf("SearchCompanies = %+v, want %+v", companies, want)
	}

	keywords, err := c.SearchKeywords(ctx, "hero")
	if err != nil {
		t.Fatalf("SearchKeywords: %v", err)
	}
	if want := []Keyword{{ID: 9715, Name: "superhero"}}; !reflect.DeepEqual(keywords, want) {
		t.Fatalf("SearchKeywords = %+v, want %+v", keywords, want)
	}

	collections, err := c.SearchCollections(ctx, "star wars")
	if err != nil {
		t.Fatalf("SearchCollections: %v", err)
	}
	if want := []Collection{{ID: 10, Name: "Star Wars Collection"}}; !reflect.DeepEqual(collections, want) {
		t.Fatalf("SearchCollections = %+v, want %+v", collections, want)
	}

	if want := []string{"/search/company?marvel", "/discover/movie?", "/search/keyword?hero", "/search/collection?star wars"}; !reflect.DeepEqual(queries(), want) {
		t.Fatalf("TMDB saw %v, want %v", queries(), want)
	}
}

// TestSearchEmptyResultIsEmptyArray pins the no-null rule for list
// endpoints: no hits marshal as [], not null.
func TestSearchEmptyResultIsEmptyArray(t *testing.T) {
	c, _ := fakeSearchTMDB(t)

	companies, err := c.SearchCompanies(t.Context(), "movie", "none")
	if err != nil {
		t.Fatalf("SearchCompanies: %v", err)
	}
	body, err := json.Marshal(companies)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "[]" {
		t.Fatalf("empty search marshals as %s, want []", body)
	}
}

func TestSearchRejectsBlankQuery(t *testing.T) {
	c, queries := fakeSearchTMDB(t)

	for _, q := range []string{"", "   "} {
		if _, err := c.SearchCompanies(t.Context(), "movie", q); !errors.Is(err, ErrInvalidParams) {
			t.Fatalf("SearchCompanies(%q) error = %v, want ErrInvalidParams", q, err)
		}
		if _, err := c.SearchKeywords(t.Context(), q); !errors.Is(err, ErrInvalidParams) {
			t.Fatalf("SearchKeywords(%q) error = %v, want ErrInvalidParams", q, err)
		}
		if _, err := c.SearchCollections(t.Context(), q); !errors.Is(err, ErrInvalidParams) {
			t.Fatalf("SearchCollections(%q) error = %v, want ErrInvalidParams", q, err)
		}
	}
	if got := queries(); len(got) != 0 {
		t.Fatalf("TMDB saw %v, want no requests for a blank query", got)
	}
}
