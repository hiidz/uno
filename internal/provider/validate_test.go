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

// TestValidateParamsLooksUpEachEntityField covers the wiring in
// ValidateParams, the lookup itself being TestCheckEntityIDs': every entity
// list reaches its own lookup and names its field when an id is unknown, and a
// field the catalog type's discover endpoint lacks (with_collection on series,
// with_networks on movie) is refused without one, since the value would
// otherwise be saved but never applied.
func TestValidateParamsLooksUpEachEntityField(t *testing.T) {
	tests := []struct {
		name, catalogType, params string
		wantField                 string // "" when the recipe passes
		wantLookup                bool
	}{
		{"known ids of every movie kind", "movie", `{"with_companies":"1","with_keywords":"1","without_companies":"1","without_keywords":"1"}`, "", true},
		{"known collection", "movie", `{"with_collection":"1"}`, "", true},
		{"known network", "series", `{"with_networks":"1"}`, "", true},
		{"unknown company", "movie", `{"with_companies":"999"}`, "with_companies", true},
		{"unknown keyword", "series", `{"with_keywords":"1,999"}`, "with_keywords", true},
		{"unknown excluded company", "movie", `{"without_companies":"1,999"}`, "without_companies", true},
		{"unknown excluded keyword", "series", `{"without_keywords":"999"}`, "without_keywords", true},
		{"unknown collection", "movie", `{"with_collection":"999"}`, "with_collection", true},
		{"unknown network", "series", `{"with_networks":"1|999"}`, "with_networks", true},
		{"collection on series", "series", `{"with_collection":"1"}`, "with_collection", false},
		{"networks on movie", "movie", `{"with_networks":"1"}`, "with_networks", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, hits := fakeEntityTMDB(t)

			err := c.ValidateParams(t.Context(), tc.catalogType, tc.params)
			if tc.wantField == "" && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if tc.wantField != "" && (!errors.Is(err, ErrInvalidParams) || !strings.Contains(err.Error(), tc.wantField)) {
				t.Fatalf("err = %v, want ErrInvalidParams naming %s", err, tc.wantField)
			}
			looked := 0
			for _, kind := range []string{"company", "keyword", "collection", "network"} {
				looked += hits("/"+kind+"/1") + hits("/"+kind+"/999")
			}
			if (looked > 0) != tc.wantLookup {
				t.Fatalf("TMDB lookups = %d, want any: %v", looked, tc.wantLookup)
			}
		})
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
// keyword include and exclude lists, for either catalog type, and on
// with_networks, which only the series recipe has: twenty ids pass, a
// twenty-first is rejected naming the field and the cap, for either separator.
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
			bothTypes := func(common TMDBCommonParams) []CatalogParams {
				return []CatalogParams{TMDBMovieParams{TMDBCommonParams: common}, TMDBTVParams{TMDBCommonParams: common}}
			}
			for _, tc := range []struct {
				field  string
				params []CatalogParams
			}{
				{"with_companies", bothTypes(TMDBCommonParams{WithCompanies: ids(n, sep)})},
				{"with_keywords", bothTypes(TMDBCommonParams{WithKeywords: ids(n, sep)})},
				{"without_companies", bothTypes(TMDBCommonParams{WithoutCompanies: ids(n, sep)})},
				{"without_keywords", bothTypes(TMDBCommonParams{WithoutKeywords: ids(n, sep)})},
				{"with_networks", []CatalogParams{TMDBTVParams{WithNetworks: ids(n, sep)}}},
			} {
				for _, p := range tc.params {
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

// fakeVocabTMDB serves the lists ValidateParams checks recipe vocabulary
// against: genre 28 for either type, language "en", watch region "US",
// watch provider 8 in the US only, and the US certification scale PG-13 and
// R. failPath answers with a server error instead. It counts every request.
func fakeVocabTMDB(t *testing.T, failPath string) (*TMDBClient, func() int) {
	t.Helper()
	var mu sync.Mutex
	total := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		total++
		mu.Unlock()

		if r.URL.Path == failPath {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch r.URL.Path {
		case "/genre/movie/list", "/genre/tv/list":
			fmt.Fprint(w, `{"genres":[{"id":28,"name":"Action"}]}`)
		case "/configuration/languages":
			fmt.Fprint(w, `[{"iso_639_1":"en","english_name":"English","name":"English"}]`)
		case "/watch/providers/regions":
			fmt.Fprint(w, `{"results":[{"iso_3166_1":"US","english_name":"United States","native_name":"United States"}]}`)
		case "/watch/providers/movie", "/watch/providers/tv":
			if r.URL.Query().Get("watch_region") != "US" {
				fmt.Fprint(w, `{"results":[]}`)
				return
			}
			fmt.Fprint(w, `{"results":[{"provider_id":8,"provider_name":"Netflix","display_priority":1}]}`)
		case "/certification/movie/list", "/certification/tv/list":
			fmt.Fprint(w, `{"certifications":{"US":[{"certification":"PG-13","order":3},{"certification":"R","order":4}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewTMDBClient("key")
	c.baseURL = srv.URL
	return c, func() int {
		mu.Lock()
		defer mu.Unlock()
		return total
	}
}

// TestValidateParamsChecksVocabularyLists covers the fields ValidateParams
// checks against TMDB's published lists — genres, language, watch region,
// watch providers and certifications: a known value passes, an unknown one
// is rejected naming its field, and a list TMDB fails to serve is returned
// as a plain error rather than a rejection.
func TestValidateParamsChecksVocabularyLists(t *testing.T) {
	tests := []struct {
		name, catalogType, params, failPath string
		wantInvalid, wantErr                bool
		wantField                           string
	}{
		{"known genres", "movie", `{"with_genres":"28|28","without_genres":"28"}`, "", false, false, ""},
		{"known genre on series", "series", `{"with_genres":"28"}`, "", false, false, ""},
		{"unknown genre", "movie", `{"with_genres":"28,99"}`, "", true, true, "with_genres"},
		{"unknown excluded genre", "series", `{"without_genres":"99"}`, "", true, true, "without_genres"},
		{"malformed genre", "movie", `{"with_genres":"action"}`, "", true, true, "with_genres"},
		{"genre list failing", "movie", `{"with_genres":"28"}`, "/genre/movie/list", false, true, ""},

		{"known language", "movie", `{"with_original_language":"en"}`, "", false, false, ""},
		{"unknown language", "series", `{"with_original_language":"xx"}`, "", true, true, "with_original_language"},
		{"language list failing", "movie", `{"with_original_language":"en"}`, "/configuration/languages", false, true, ""},

		{"known region", "movie", `{"watch_region":"US"}`, "", false, false, ""},
		{"unknown region", "series", `{"watch_region":"ZZ"}`, "", true, true, "watch_region"},
		{"region list failing", "movie", `{"watch_region":"US"}`, "/watch/providers/regions", false, true, ""},

		{"known provider", "movie", `{"watch_region":"US","with_watch_providers":"8"}`, "", false, false, ""},
		{"known provider on series", "series", `{"watch_region":"US","with_watch_providers":"8|8"}`, "", false, false, ""},
		{"unknown provider", "movie", `{"watch_region":"US","with_watch_providers":"8,9"}`, "", true, true, "with_watch_providers"},
		{"provider list failing", "movie", `{"watch_region":"US","with_watch_providers":"8"}`, "/watch/providers/movie", false, true, ""},

		{"known certification", "movie", `{"certification_country":"US","certification":"PG-13"}`, "", false, false, ""},
		{"known certification range", "series", `{"certification_country":"US","certification_gte":"PG-13","certification_lte":"R"}`, "", false, false, ""},
		{"country alone", "movie", `{"certification_country":"US"}`, "", false, false, ""},
		{"unknown country", "movie", `{"certification_country":"ZZ","certification":"PG-13"}`, "", true, true, "certification_country"},
		{"certification without country", "movie", `{"certification":"PG-13"}`, "", true, true, "certification_country"},
		{"unknown certification", "movie", `{"certification_country":"US","certification":"X"}`, "", true, true, "certification "},
		{"unknown lower bound", "series", `{"certification_country":"US","certification_gte":"X"}`, "", true, true, "certification_gte"},
		{"unknown upper bound", "movie", `{"certification_country":"US","certification_lte":"X"}`, "", true, true, "certification_lte"},
		{"certification list failing", "movie", `{"certification_country":"US"}`, "/certification/movie/list", false, true, ""},

		{"undecodable params", "movie", `{"with_genres":28}`, "", true, true, "decode"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := fakeVocabTMDB(t, tc.failPath)

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

// TestValidateParamsFetchesNothingForBareRecipe pins that a recipe setting
// none of the vocabulary fields costs no TMDB round trip at all.
func TestValidateParamsFetchesNothingForBareRecipe(t *testing.T) {
	c, total := fakeVocabTMDB(t, "")

	if err := c.ValidateParams(t.Context(), "movie", `{"sort_by":"popularity.desc"}`); err != nil {
		t.Fatalf("ValidateParams: %v", err)
	}
	if n := total(); n != 0 {
		t.Fatalf("TMDB requests = %d, want 0", n)
	}
}

// TestValidateParamsRejectsUnknownCatalogType pins that the catalog type is
// checked before anything else.
func TestValidateParamsRejectsUnknownCatalogType(t *testing.T) {
	c, total := fakeVocabTMDB(t, "")

	err := c.ValidateParams(t.Context(), "anime", `{"with_genres":"28"}`)
	if !errors.Is(err, ErrInvalidCatalogType) {
		t.Fatalf("err = %v, want ErrInvalidCatalogType", err)
	}
	if n := total(); n != 0 {
		t.Fatalf("TMDB requests = %d, want 0", n)
	}
}

// A range whose low end is above its high end can't match any title, so the
// server refuses it as the builder does. Zero is "unset": a range with no high
// end, or one whose two ends are equal, is fine.
func TestValidateRefusesAnInvertedRange(t *testing.T) {
	for _, tc := range []struct {
		name    string
		params  TMDBCommonParams
		wantErr string
	}{
		{"vote_average inverted", TMDBCommonParams{VoteAverageGte: 8, VoteAverageLte: 6}, "vote_average_gte cannot be higher than vote_average_lte"},
		{"vote_count inverted", TMDBCommonParams{VoteCountGte: 500, VoteCountLte: 100}, "vote_count_gte cannot be higher than vote_count_lte"},
		{"runtime inverted", TMDBCommonParams{WithRuntimeGte: 120, WithRuntimeLte: 90}, "with_runtime_gte cannot be higher than with_runtime_lte"},
		{"equal ends", TMDBCommonParams{VoteAverageGte: 7, VoteAverageLte: 7, WithRuntimeGte: 90, WithRuntimeLte: 90}, ""},
		{"a low end alone", TMDBCommonParams{VoteAverageGte: 7, VoteCountGte: 100, WithRuntimeGte: 90}, ""},
		{"a high end alone", TMDBCommonParams{VoteAverageLte: 7, VoteCountLte: 100, WithRuntimeLte: 90}, ""},
		{"an ordered range", TMDBCommonParams{VoteAverageGte: 6, VoteAverageLte: 8}, ""},
	} {
		for kind, validate := range map[string]func() error{
			"movie": TMDBMovieParams{TMDBCommonParams: tc.params}.Validate,
			"tv":    TMDBTVParams{TMDBCommonParams: tc.params}.Validate,
		} {
			err := validate()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("%s %s: Validate = %v, want nil", kind, tc.name, err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Errorf("%s %s: Validate = %v, want it to say %q", kind, tc.name, err, tc.wantErr)
			}
		}
	}
}

// A fixed date is a YYYY-MM-DD date that exists, and a range's start is not
// after its end; each type's own two fields are checked, and an empty bound is
// unset.
func TestValidateChecksFixedDates(t *testing.T) {
	for _, tc := range []struct {
		name     string
		from, to string
		wantErr  string
	}{
		{"an ordered range", "2020-01-01", "2020-12-31", ""},
		{"the same day", "2020-06-15", "2020-06-15", ""},
		{"a start alone", "2020-01-01", "", ""},
		{"an end alone", "", "2020-12-31", ""},
		{"start after end", "2021-01-01", "2020-12-31", "cannot be after"},
		{"a start in another form", "01/02/2020", "", "must be a date as YYYY-MM-DD"},
		{"a start with a time", "2020-01-01T00:00:00Z", "", "must be a date as YYYY-MM-DD"},
		{"an end that isn't a date", "", "soon", "must be a date as YYYY-MM-DD"},
		{"a day that doesn't exist", "2021-02-30", "", "must be a date as YYYY-MM-DD"},
		{"an unpadded month", "2020-1-05", "", "must be a date as YYYY-MM-DD"},
	} {
		movie := TMDBMovieParams{PrimaryReleaseDateGte: tc.from, PrimaryReleaseDateLte: tc.to}.Validate()
		tv := TMDBTVParams{FirstAirDateGte: tc.from, FirstAirDateLte: tc.to}.Validate()
		for kind, err := range map[string]error{"movie": movie, "tv": tv} {
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("%s %s: Validate = %v, want nil", kind, tc.name, err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Errorf("%s %s: Validate = %v, want it to say %q", kind, tc.name, err, tc.wantErr)
			}
		}
	}
}
