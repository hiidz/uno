package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// commonQuery translates the fields both recipe types share into TMDB's
// literal query params. The translation is done field-by-field (here and in
// each type's DiscoverQuery) rather than as a generic json-roundtrip dump: our
// own storage tags are underscore-only (vote_average_gte) but TMDB's real
// range params use a dot (vote_average.gte) — dumping the struct directly
// would silently produce query keys TMDB doesn't recognize and the filter
// would just never apply.
func commonQuery(p TMDBCommonParams) url.Values {
	q := url.Values{}
	setIf(q, "sort_by", p.SortBy)
	setIf(q, "with_genres", p.WithGenres)
	setIf(q, "without_genres", p.WithoutGenres)
	setIf(q, "with_original_language", p.WithOriginalLanguage)
	setIf(q, "with_watch_providers", p.WithWatchProviders)
	setIf(q, "watch_region", p.WatchRegion)
	setIf(q, "with_companies", p.WithCompanies)
	setIf(q, "with_keywords", p.WithKeywords)
	setIf(q, "certification", p.Certification)
	setIf(q, "certification.gte", p.CertificationGte)
	setIf(q, "certification.lte", p.CertificationLte)
	setIf(q, "certification_country", p.CertificationCountry)
	setFloatIf(q, "vote_average.gte", p.VoteAverageGte)
	setFloatIf(q, "vote_average.lte", p.VoteAverageLte)
	setIntIf(q, "vote_count.gte", p.VoteCountGte)
	setIntIf(q, "vote_count.lte", p.VoteCountLte)
	setIntIf(q, "with_runtime.gte", p.WithRuntimeGte)
	setIntIf(q, "with_runtime.lte", p.WithRuntimeLte)
	return q
}

// DiscoverQuery satisfies CatalogParams for a /discover/movie recipe.
// with_collection is not sent: /discover/movie ignores it, and a collection
// recipe never reaches discover (see collectionItems).
func (p TMDBMovieParams) DiscoverQuery() url.Values {
	q := commonQuery(p.TMDBCommonParams)
	setIf(q, "primary_release_date.gte", p.PrimaryReleaseDateGte)
	setIf(q, "primary_release_date.lte", p.PrimaryReleaseDateLte)
	if p.ReleasedWithinDays > 0 {
		q.Set("primary_release_date.gte", daysAgo(p.ReleasedWithinDays))
	}
	return q
}

// DiscoverQuery satisfies CatalogParams for a /discover/tv recipe.
func (p TMDBTVParams) DiscoverQuery() url.Values {
	q := commonQuery(p.TMDBCommonParams)
	setIf(q, "first_air_date.gte", p.FirstAirDateGte)
	setIf(q, "first_air_date.lte", p.FirstAirDateLte)
	if p.AiredWithinDays > 0 {
		q.Set("air_date.gte", daysAgo(p.AiredWithinDays))
	}
	return q
}

// GenreExtraAll is the first option of a required genre extra: the value a
// client sends by default, meaning "no genre filter".
const GenreExtraAll = "All"

// GenreExtraOptions returns the genres a client may pick to narrow a stored
// catalog: TMDB's genre list for its type, minus those a pick can't narrow
// by (see genreChoices). The manifest advertises their names, and
// FetchCatalogPage resolves a picked name back to its id through the same
// list.
func (c *TMDBClient) GenreExtraOptions(ctx context.Context, catalogType, paramsJSON string) ([]Genre, error) {
	var p struct {
		WithGenres    string `json:"with_genres"`
		WithoutGenres string `json:"without_genres"`
	}
	if err := json.Unmarshal([]byte(paramsJSON), &p); err != nil {
		return nil, fmt.Errorf("provider: decode genre filters: %w", err)
	}
	genres, err := c.Genres(ctx, catalogType)
	if err != nil {
		return nil, err
	}
	return genreChoices(genres, p.WithGenres, p.WithoutGenres), nil
}

// genreChoices filters genres down to the ones a pick narrows the recipe by.
// With an OR (pipe) with_genres, only that list's genres: a pick replaces
// the list (see applyGenreExtra), and TMDB can't express "(A or B) and C".
// Otherwise every genre except those the recipe already requires or
// excludes — picking one of those changes nothing or empties the row.
func genreChoices(genres []Genre, withGenres, withoutGenres string) []Genre {
	isOr := strings.Contains(withGenres, "|")
	with := genreIDSet(withGenres)
	without := genreIDSet(withoutGenres)
	out := make([]Genre, 0, len(genres))
	for _, g := range genres {
		if isOr && !with[g.ID] {
			continue
		}
		if !isOr && (with[g.ID] || without[g.ID]) {
			continue
		}
		out = append(out, g)
	}
	return out
}

func genreIDSet(list string) map[int]bool {
	set := map[int]bool{}
	for _, part := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == '|' }) {
		if id, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			set[id] = true
		}
	}
	return set
}

// applyGenrePick narrows q to genre, a name picked from the recipe's
// GenreExtraOptions. "", GenreExtraAll, or a name not in that list leaves q
// unfiltered.
func (c *TMDBClient) applyGenrePick(ctx context.Context, q url.Values, catalogType, paramsJSON, genre string) error {
	id, ok, err := c.pickedGenreID(ctx, catalogType, paramsJSON, genre)
	if ok {
		applyGenreExtra(q, id)
	}
	return err
}

// pickedGenreID resolves genre, a name picked from the recipe's
// GenreExtraOptions, to its TMDB id. ok is false for "", GenreExtraAll, or a
// name not in that list.
func (c *TMDBClient) pickedGenreID(ctx context.Context, catalogType, paramsJSON, genre string) (id int, ok bool, err error) {
	if genre == "" || genre == GenreExtraAll {
		return 0, false, nil
	}
	options, err := c.GenreExtraOptions(ctx, catalogType, paramsJSON)
	if err != nil {
		return 0, false, err
	}
	for _, g := range options {
		if g.Name == genre {
			return g.ID, true, nil
		}
	}
	return 0, false, nil
}

// applyGenreExtra narrows a discover query to one picked genre id: ANDed onto
// an AND (comma) with_genres, and replacing an OR (pipe) one — genreChoices
// only offers an OR list's own genres, so the replacement narrows it.
func applyGenreExtra(q url.Values, genreID int) {
	id := strconv.Itoa(genreID)
	existing := q.Get("with_genres")
	if existing == "" || strings.Contains(existing, "|") {
		q.Set("with_genres", id)
		return
	}
	q.Set("with_genres", existing+","+id)
}

func daysAgo(days int) string {
	return time.Now().AddDate(0, 0, -days).Format("2006-01-02")
}

func setIf(q url.Values, key, val string) {
	if val != "" {
		q.Set(key, val)
	}
}

func setIntIf(q url.Values, key string, val int) {
	if val != 0 {
		q.Set(key, strconv.Itoa(val))
	}
}

func setFloatIf(q url.Values, key string, val float64) {
	if val != 0 {
		q.Set(key, strconv.FormatFloat(val, 'f', -1, 64))
	}
}
