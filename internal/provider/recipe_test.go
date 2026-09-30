package provider

import (
	"strings"
	"testing"
)

// The canonical form keeps a type's known keys only, drops zero values, sorts
// the keys and writes numbers the way Go does, whatever order, spacing or
// spelling the params arrived in.
func TestCanonicalParams(t *testing.T) {
	for _, tc := range []struct {
		name, catalogType, params, want string
	}{
		{"sorted and compact", "movie", `{
			"with_genres": "28",
			"sort_by":     "popularity.desc"
		}`, `{"sort_by":"popularity.desc","with_genres":"28"}`},
		{"zero values dropped", "series", `{"sort_by":"","vote_count_gte":0,"randomized":false,"aired_within_days":7}`, `{"aired_within_days":7}`},
		{"unknown keys dropped", "movie", `{"sort_by":"revenue.desc","endpoint":"/discover/movie"}`, `{"sort_by":"revenue.desc"}`},
		{"another type's key dropped", "movie", `{"with_networks":"213","with_keywords":"9748"}`, `{"with_keywords":"9748"}`},
		{"numbers as Go writes them", "movie", `{"vote_average_gte":7.50,"vote_average_lte":1e1}`, `{"vote_average_gte":7.5,"vote_average_lte":10}`},
		{"empty", "series", `{}`, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CanonicalParams(tc.catalogType, "tmdb", tc.params)
			if err != nil {
				t.Fatalf("CanonicalParams: %v", err)
			}
			if got != tc.want {
				t.Errorf("CanonicalParams = %s, want %s", got, tc.want)
			}
		})
	}
}

// Params that don't decode as the type's recipe, a type Uno doesn't know,
// and a provider this package has no recipes for have no canonical form.
func TestCanonicalParamsRefusals(t *testing.T) {
	for _, tc := range []struct{ catalogType, catalogProvider, params, want string }{
		{"documentary", "tmdb", `{}`, "invalid catalog type"},
		{"movie", "tmdb", `{"vote_count_gte":"500"}`, "decode movie params"},
		{"movie", "tmdb", `not json`, "decode movie params"},
		{"movie", "letterboxd", `{}`, `no recipes for provider "letterboxd"`},
	} {
		if _, err := CanonicalParams(tc.catalogType, tc.catalogProvider, tc.params); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("CanonicalParams(%s, %s, %s) = %v, want an error containing %q", tc.catalogType, tc.catalogProvider, tc.params, err, tc.want)
		}
	}
	if err := SameRecipe("movie", "letterboxd", `{}`, `{}`); err == nil || !strings.Contains(err.Error(), "no recipes for provider") {
		t.Errorf("SameRecipe for another provider = %v, want its refusal", err)
	}
}

// SameRecipe accepts params that differ only in form, and names the first
// thing that differs otherwise, TMDB-side or in what the manifest offers.
func TestSameRecipe(t *testing.T) {
	for _, tc := range []struct {
		name, catalogType, before, after, want string
	}{
		{"canonical form", "movie", `{"with_genres":"28","sort_by":"popularity.desc","legacy":1}`, `{"sort_by":"popularity.desc","with_genres":"28"}`, ""},
		{"discover query", "movie", `{"sort_by":"popularity.desc"}`, `{"sort_by":"revenue.desc"}`, "discover query differs"},
		{"shuffle", "series", `{"randomized":true}`, `{}`, "shuffle differs"},
		{"collection", "movie", `{"with_collection":"10"}`, `{"with_collection":"87096"}`, "collection differs"},
		{"genre options by OR list", "movie", `{"with_genres":"28|12"}`, `{"with_genres":"28,12"}`, "discover query differs"},
		{"rolling window", "series", `{"aired_within_days":30}`, `{"aired_within_days":30}`, ""},
		{"before undecodable", "movie", `{"vote_count_gte":"x"}`, `{}`, "before:"},
		{"after undecodable", "movie", `{}`, `[]`, "after:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := SameRecipe(tc.catalogType, "tmdb", tc.before, tc.after)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("SameRecipe = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("SameRecipe = %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

// The genre options SameRecipe compares tell an OR list from an AND one, and
// see an excluded genre.
func TestGenreOptionIDs(t *testing.T) {
	a := recipeFacts{withGenres: "28|12"}
	b := recipeFacts{withGenres: "28,12"}
	if genreOptionIDs(a, b) == genreOptionIDs(b, a) {
		t.Errorf("genre options of an OR and an AND list both %s, want them to differ", genreOptionIDs(a, b))
	}
	c := recipeFacts{withoutGenres: "27"}
	if genreOptionIDs(c, recipeFacts{}) == genreOptionIDs(recipeFacts{}, c) {
		t.Errorf("genre options with and without an excluded genre both %s, want them to differ", genreOptionIDs(c, recipeFacts{}))
	}
}

// ValidateRecipe runs the checks that need no network: a recipe that
// decodes and whose params pass Validate is fine; an unknown provider, an
// unknown type, params that don't decode and a rule Validate refuses are
// not.
func TestValidateRecipe(t *testing.T) {
	if err := ValidateRecipe("movie", "tmdb", `{"sort_by":"popularity.desc"}`); err != nil {
		t.Errorf("ValidateRecipe = %v, want nil", err)
	}
	for _, tc := range []struct{ catalogType, catalogProvider, params string }{
		{"movie", "letterboxd", `{}`},
		{"anime", "tmdb", `{}`},
		{"movie", "tmdb", `[]`},
		{"movie", "tmdb", `{"sort_by":"nonsense"}`},
	} {
		if err := ValidateRecipe(tc.catalogType, tc.catalogProvider, tc.params); err == nil {
			t.Errorf("ValidateRecipe(%s, %s, %s) = nil, want an error", tc.catalogType, tc.catalogProvider, tc.params)
		}
	}
}
