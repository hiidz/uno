package provider

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeEntityTMDB serves /company/{id}, /keyword/{id}, /collection/{id} and
// /network/{id}: id 1 exists, id 500 fails with a server error, anything else
// is a 404. It counts every request per path.
func fakeEntityTMDB(t *testing.T) (*TMDBClient, func(path string) int) {
	t.Helper()
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()

		switch r.URL.Path {
		case "/company/1":
			fmt.Fprint(w, `{"id":1,"name":"Lucasfilm Ltd.","origin_country":"US"}`)
		case "/keyword/1":
			fmt.Fprint(w, `{"id":1,"name":"superhero"}`)
		case "/collection/1":
			fmt.Fprint(w, `{"id":1,"name":"Star Wars Collection","parts":[]}`)
		case "/network/1":
			fmt.Fprint(w, `{"id":1,"name":"Fuji TV","origin_country":"JP"}`)
		case "/company/500", "/keyword/500", "/collection/500", "/network/500":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewTMDBClient("key")
	c.baseURL = srv.URL
	return c, func(path string) int {
		mu.Lock()
		defer mu.Unlock()
		return hits[path]
	}
}

func TestCheckEntityIDs(t *testing.T) {
	tests := []struct {
		name        string
		list        string
		wantInvalid bool // the error wraps ErrInvalidParams
		wantErr     bool // any error at all
		wantHits    int  // requests TMDB sees
	}{
		{name: "empty list", list: "", wantHits: 0},
		{name: "known id", list: "1", wantHits: 1},
		{name: "known ids, AND and OR", list: "1,1|1", wantHits: 1},
		{name: "malformed id", list: "1,abc", wantInvalid: true, wantErr: true, wantHits: 0},
		{name: "non-positive id", list: "0", wantInvalid: true, wantErr: true, wantHits: 0},
		{name: "unknown id", list: "1|999", wantInvalid: true, wantErr: true, wantHits: 2},
		{name: "TMDB failing", list: "500", wantErr: true, wantHits: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, hits := fakeEntityTMDB(t)

			err := checkEntityIDs(t.Context(), "with_companies", tc.list, "company", c.Company)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error: %v", err, tc.wantErr)
			}
			if got := errors.Is(err, ErrInvalidParams); got != tc.wantInvalid {
				t.Fatalf("errors.Is(err, ErrInvalidParams) = %v, want %v (err: %v)", got, tc.wantInvalid, err)
			}

			total := hits("/company/1") + hits("/company/999") + hits("/company/500")
			if total != tc.wantHits {
				t.Fatalf("TMDB hit %d times, want %d", total, tc.wantHits)
			}
		})
	}
}

// TestValidateParamsChecksCompaniesAndKeywords covers the wiring in
// ValidateParams: the include and exclude lists of both kinds reach their own
// lookup, for either catalog type.
func TestValidateParamsChecksCompaniesAndKeywords(t *testing.T) {
	tests := []struct {
		name, catalogType, params string
		wantInvalid, wantErr      bool
		wantField                 string
	}{
		{"known company and keyword", "movie", `{"with_companies":"1","with_keywords":"1"}`, false, false, ""},
		{"known on series", "series", `{"with_companies":"1|1","with_keywords":"1,1"}`, false, false, ""},
		{"unknown company", "movie", `{"with_companies":"999"}`, true, true, "with_companies"},
		{"unknown keyword", "series", `{"with_keywords":"1,999"}`, true, true, "with_keywords"},
		{"keyword lookup failing", "movie", `{"with_keywords":"500"}`, false, true, ""},
		{"known exclusions", "series", `{"without_companies":"1","without_keywords":"1,1"}`, false, false, ""},
		{"unknown excluded company", "movie", `{"without_companies":"1,999"}`, true, true, "without_companies"},
		{"unknown excluded keyword", "series", `{"without_keywords":"999"}`, true, true, "without_keywords"},
		{"excluded company lookup failing", "movie", `{"without_companies":"500"}`, false, true, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := fakeEntityTMDB(t)

			err := c.ValidateParams(t.Context(), tc.catalogType, tc.params)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error: %v", err, tc.wantErr)
			}
			if got := errors.Is(err, ErrInvalidParams); got != tc.wantInvalid {
				t.Fatalf("errors.Is(err, ErrInvalidParams) = %v, want %v (err: %v)", got, tc.wantInvalid, err)
			}
			if tc.wantField != "" && !strings.Contains(err.Error(), tc.wantField) {
				t.Fatalf("err = %v, want it to name %s", err, tc.wantField)
			}
		})
	}
}

// TestValidateParamsChecksCollection covers with_collection in
// ValidateParams: a movie recipe's id is looked up on TMDB, and a series
// recipe carrying it is rejected without a lookup, since /discover/tv has no
// such filter and the value would otherwise be saved but never applied.
func TestValidateParamsChecksCollection(t *testing.T) {
	tests := []struct {
		name, catalogType, params string
		wantInvalid, wantErr      bool
		wantHits                  int
	}{
		{"known collection", "movie", `{"with_collection":"1"}`, false, false, 1},
		{"unknown collection", "movie", `{"with_collection":"999"}`, true, true, 1},
		{"malformed collection", "movie", `{"with_collection":"abc"}`, true, true, 0},
		{"collection lookup failing", "movie", `{"with_collection":"500"}`, false, true, 1},
		{"collection on series", "series", `{"with_collection":"1"}`, true, true, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, hits := fakeEntityTMDB(t)

			err := c.ValidateParams(t.Context(), tc.catalogType, tc.params)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error: %v", err, tc.wantErr)
			}
			if got := errors.Is(err, ErrInvalidParams); got != tc.wantInvalid {
				t.Fatalf("errors.Is(err, ErrInvalidParams) = %v, want %v (err: %v)", got, tc.wantInvalid, err)
			}
			if tc.wantInvalid && !strings.Contains(err.Error(), "with_collection") {
				t.Fatalf("err = %v, want it to name with_collection", err)
			}
			total := hits("/collection/1") + hits("/collection/999") + hits("/collection/500")
			if total != tc.wantHits {
				t.Fatalf("TMDB hit %d times, want %d", total, tc.wantHits)
			}
		})
	}
}

// TestValidateParamsChecksNetworks covers with_networks in ValidateParams: a
// series recipe's ids are looked up on TMDB, and a movie recipe carrying them
// is rejected without a lookup, since /discover/movie has no such filter and
// the value would otherwise be saved but never applied.
func TestValidateParamsChecksNetworks(t *testing.T) {
	tests := []struct {
		name, catalogType, params string
		wantInvalid, wantErr      bool
		wantHits                  int
	}{
		{"known networks, AND and OR", "series", `{"with_networks":"1|1,1"}`, false, false, 1},
		{"unknown network", "series", `{"with_networks":"1|999"}`, true, true, 2},
		{"malformed network", "series", `{"with_networks":"abc"}`, true, true, 0},
		{"network lookup failing", "series", `{"with_networks":"500"}`, false, true, 1},
		{"networks on movie", "movie", `{"with_networks":"1"}`, true, true, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, hits := fakeEntityTMDB(t)

			err := c.ValidateParams(t.Context(), tc.catalogType, tc.params)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error: %v", err, tc.wantErr)
			}
			if got := errors.Is(err, ErrInvalidParams); got != tc.wantInvalid {
				t.Fatalf("errors.Is(err, ErrInvalidParams) = %v, want %v (err: %v)", got, tc.wantInvalid, err)
			}
			if tc.wantInvalid && !strings.Contains(err.Error(), "with_networks") {
				t.Fatalf("err = %v, want it to name with_networks", err)
			}
			total := hits("/network/1") + hits("/network/999") + hits("/network/500")
			if total != tc.wantHits {
				t.Fatalf("TMDB hit %d times, want %d", total, tc.wantHits)
			}
		})
	}
}

// TestTVValidateCapsNetworks covers the no-network cap on with_networks,
// which lives on the series recipe alone: twenty ids pass, a twenty-first is
// rejected naming the field and the cap, for either separator.
func TestTVValidateCapsNetworks(t *testing.T) {
	for _, sep := range []string{",", "|"} {
		for _, n := range []int{maxEntityIDs, maxEntityIDs + 1} {
			parts := make([]string, n)
			for i := range parts {
				parts[i] = fmt.Sprint(i + 1)
			}
			err := TMDBTVParams{WithNetworks: strings.Join(parts, sep)}.Validate()
			if n <= maxEntityIDs {
				if err != nil {
					t.Fatalf("with_networks with %d ids: %v, want accepted", n, err)
				}
				continue
			}
			if err == nil || !strings.Contains(err.Error(), "with_networks") || !strings.Contains(err.Error(), "20") {
				t.Fatalf("with_networks with %d ids: %v, want an error naming with_networks and the cap", n, err)
			}
		}
	}
}

// TestMovieValidateRejectsCollectionList covers the no-network half of
// with_collection: TMDB takes exactly one collection id, so an AND or OR list
// is rejected before any lookup.
func TestMovieValidateRejectsCollectionList(t *testing.T) {
	for _, tc := range []struct {
		value   string
		wantErr bool
	}{
		{"", false},
		{"10", false},
		{"10,11", true},
		{"10|11", true},
	} {
		err := TMDBMovieParams{WithCollection: tc.value}.Validate()
		if (err != nil) != tc.wantErr {
			t.Fatalf("Validate(with_collection %q) = %v, want error: %v", tc.value, err, tc.wantErr)
		}
	}
}

// TestValidateCapsEntityIDLists covers the no-network cap on the company and
// keyword include and exclude lists: twenty ids pass, a twenty-first is rejected naming the
// field and the cap, for either separator and either catalog type.
func TestValidateCapsEntityIDLists(t *testing.T) {
	ids := func(n int, sep string) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = fmt.Sprint(i + 1)
		}
		return strings.Join(parts, sep)
	}

	for _, sep := range []string{",", "|"} {
		for _, n := range []int{maxEntityIDs, maxEntityIDs + 1} {
			for _, tc := range []struct {
				field  string
				common TMDBCommonParams
			}{
				{"with_companies", TMDBCommonParams{WithCompanies: ids(n, sep)}},
				{"with_keywords", TMDBCommonParams{WithKeywords: ids(n, sep)}},
				{"without_companies", TMDBCommonParams{WithoutCompanies: ids(n, sep)}},
				{"without_keywords", TMDBCommonParams{WithoutKeywords: ids(n, sep)}},
			} {
				for _, p := range []CatalogParams{
					TMDBMovieParams{TMDBCommonParams: tc.common},
					TMDBTVParams{TMDBCommonParams: tc.common},
				} {
					err := p.Validate()
					if n <= maxEntityIDs {
						if err != nil {
							t.Fatalf("%T %s with %d ids: %v, want accepted", p, tc.field, n, err)
						}
						continue
					}
					if err == nil || !strings.Contains(err.Error(), tc.field) || !strings.Contains(err.Error(), "20") {
						t.Fatalf("%T %s with %d ids: %v, want an error naming %s and the cap", p, tc.field, n, err, tc.field)
					}
				}
			}
		}
	}
}
