package provider

import (
	"net/url"
	"reflect"
	"testing"
)

func TestApplyGenreExtra(t *testing.T) {
	tests := []struct {
		name       string
		withGenres string
		want       string
	}{
		{"no recipe genres", "", "28"},
		{"ANDed onto a single genre", "12", "12,28"},
		{"ANDed onto an AND list", "12,16", "12,16,28"},
		{"replaces an OR list", "28|35", "28"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := url.Values{}
			if tt.withGenres != "" {
				q.Set("with_genres", tt.withGenres)
			}
			applyGenreExtra(q, 28)
			if got := q.Get("with_genres"); got != tt.want {
				t.Fatalf("with_genres = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenreChoices(t *testing.T) {
	all := []Genre{{12, "Adventure"}, {28, "Action"}, {35, "Comedy"}, {27, "Horror"}}
	ids := func(genres []Genre) []int {
		out := []int{}
		for _, g := range genres {
			out = append(out, g.ID)
		}
		return out
	}

	tests := []struct {
		name                      string
		withGenres, withoutGenres string
		want                      []int
	}{
		{"no recipe genres offers all", "", "", []int{12, 28, 35, 27}},
		{"drops AND-required genres", "28,35", "", []int{12, 27}},
		{"drops excluded genres", "", "27", []int{12, 28, 35}},
		{"OR list offers only its own genres", "28|35", "", []int{28, 35}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ids(genreChoices(all, tt.withGenres, tt.withoutGenres))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("genreChoices = %v, want %v", got, tt.want)
			}
		})
	}
}
