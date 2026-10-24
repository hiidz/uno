package provider

import (
	"context"

	"github.com/hiidz/uno/internal/jsonwire"
)

// Person is one TMDB person — with_cast and with_crew take ID, and the
// picker shows Name over the headshot at ProfilePath, TMDB's raw image path.
type Person struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	ProfilePath        string `json:"profile_path"`
	KnownForDepartment string `json:"known_for_department"`
}

// PersonMatch is one person search result: a Person and the title of the
// first work TMDB lists them as known for, "" when it lists none.
type PersonMatch struct {
	Person
	KnownFor string `json:"known_for"`
}

// personSearchResult is one row of /search/person as TMDB sends it. A
// known-for entry names a movie by title and a series by name.
type personSearchResult struct {
	Person
	KnownFor []struct {
		Title string `json:"title"`
		Name  string `json:"name"`
	} `json:"known_for"`
}

// Person returns the TMDB person with id, memoized. It serves both recipe
// validation's existence check and the picker's id-to-name resolution. An id
// TMDB doesn't have wraps ErrNotFound.
func (c *TMDBClient) Person(ctx context.Context, id int) (Person, error) {
	return entityByID(ctx, c, c.people, "/person/%d", id)
}

// SearchPeople returns the first page of TMDB's person search for query, in
// TMDB's order. Not cached: results vary with every keystroke the picker
// sends.
func (c *TMDBClient) SearchPeople(ctx context.Context, query string) ([]PersonMatch, error) {
	results, err := searchEntities[personSearchResult](ctx, c, "/search/person", query)
	if err != nil {
		return nil, err
	}
	matches := make([]PersonMatch, 0, len(results))
	for _, r := range results {
		match := PersonMatch{Person: r.Person}
		if len(r.KnownFor) > 0 {
			match.KnownFor = r.KnownFor[0].Title
			if match.KnownFor == "" {
				match.KnownFor = r.KnownFor[0].Name
			}
		}
		matches = append(matches, match)
	}
	return jsonwire.OrEmpty(matches), nil
}
