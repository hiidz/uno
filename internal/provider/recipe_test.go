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
}
