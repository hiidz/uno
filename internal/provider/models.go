package provider

import (
	"errors"
	"fmt"
)

// Genre is a single TMDB genre id/name pair.
type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type genreListResponse struct {
	Genres []Genre `json:"genres"`
}

// Certification is one entry in a country's age-rating scale, e.g. US movie
// "PG-13". Order is the scale's own ranking (lowest to highest) — it's what
// certification_gte/lte compare against on TMDB's side, so a caller building
// a min/max picker should sort by it rather than alphabetically.
type Certification struct {
	Certification string `json:"certification"`
	Meaning       string `json:"meaning"`
	Order         int    `json:"order"`
}

type certificationListResponse struct {
	Certifications map[string][]Certification `json:"certifications"`
}

// BaseParams holds catalog behavior that isn't specific to any provider —
// it wouldn't make sense as a TMDB query param because TMDB has no concept
// of it; it's app-level logic layered on top of whatever provider is used.
type BaseParams struct {
	Randomized bool `json:"randomized,omitempty"`
}

// TMDBCommonParams holds fields identical across /discover/movie and
// /discover/tv. Whether a catalog is movie or tv comes from the catalog
// row itself (kind/type column), not from these structs.
type TMDBCommonParams struct {
	BaseParams

	SortBy string `json:"sort_by,omitempty"`

	WithGenres    string `json:"with_genres,omitempty"` // comma (AND) or pipe (OR) separated TMDB genre ids
	WithoutGenres string `json:"without_genres,omitempty"`

	WithOriginalLanguage string `json:"with_original_language,omitempty"`

	VoteAverageGte float64 `json:"vote_average_gte,omitempty"`
	VoteAverageLte float64 `json:"vote_average_lte,omitempty"`

	VoteCountGte int `json:"vote_count_gte,omitempty"`
	VoteCountLte int `json:"vote_count_lte,omitempty"`

	WithRuntimeGte int `json:"with_runtime_gte,omitempty"`
	WithRuntimeLte int `json:"with_runtime_lte,omitempty"`

	WithWatchProviders string `json:"with_watch_providers,omitempty"` // comma/pipe separated provider ids
	WatchRegion        string `json:"watch_region,omitempty"`         // required alongside WithWatchProviders

	Certification        string `json:"certification,omitempty"`
	CertificationGte     string `json:"certification_gte,omitempty"`
	CertificationLte     string `json:"certification_lte,omitempty"`
	CertificationCountry string `json:"certification_country,omitempty"` // required alongside any Certification field
}

// TMDBMovieParams is the recipe for a /discover/movie-backed catalog.
type TMDBMovieParams struct {
	TMDBCommonParams

	// Fixed range: movies released between these dates (YYYY-MM-DD).
	PrimaryReleaseDateGte string `json:"primary_release_date_gte,omitempty"`
	PrimaryReleaseDateLte string `json:"primary_release_date_lte,omitempty"`

	// Dynamic: movies released in the last N days. Converted to
	// primary_release_date_gte = today-N at query time, every render.
	// Mutually exclusive with the fixed fields above — enforced in Validate.
	ReleasedWithinDays int `json:"released_within_days,omitempty"`
}

// TMDBTVParams is the recipe for a /discover/tv-backed catalog.
type TMDBTVParams struct {
	TMDBCommonParams

	// Fixed range: shows that premiered between these dates (YYYY-MM-DD).
	FirstAirDateGte string `json:"first_air_date_gte,omitempty"`
	FirstAirDateLte string `json:"first_air_date_lte,omitempty"`

	// Dynamic: shows with an episode aired in the last N days — a
	// different axis from premiere date (a show from 2015 can still
	// match if it aired last week). Converted to air_date_gte = today-N
	// at query time, every render. Mutually exclusive with the fixed
	// fields above — enforced in Validate.
	AiredWithinDays int `json:"aired_within_days,omitempty"`
}

// validMovieSortValues is TMDB's complete sort_by enum for /discover/movie.
var validMovieSortValues = map[string]bool{
	"popularity.asc": true, "popularity.desc": true,
	"vote_average.asc": true, "vote_average.desc": true,
	"vote_count.asc": true, "vote_count.desc": true,
	"primary_release_date.asc": true, "primary_release_date.desc": true,
	"original_title.asc": true, "original_title.desc": true,
	"title.asc": true, "title.desc": true,
	"revenue.asc": true, "revenue.desc": true,
}

// validTVSortValues is TMDB's complete sort_by enum for /discover/tv.
// Note: no revenue or title.asc/desc — TV uses name.asc/desc instead.
var validTVSortValues = map[string]bool{
	"popularity.asc": true, "popularity.desc": true,
	"vote_average.asc": true, "vote_average.desc": true,
	"vote_count.asc": true, "vote_count.desc": true,
	"first_air_date.asc": true, "first_air_date.desc": true,
	"name.asc": true, "name.desc": true,
	"original_name.asc": true, "original_name.desc": true,
}

// validate checks the cross-field rules shared by movie and tv params —
// the "required together" pairs that don't differ per catalog type.
func (p TMDBCommonParams) validate() error {
	hasCertFilter := p.Certification != "" || p.CertificationGte != "" || p.CertificationLte != ""
	if hasCertFilter && p.CertificationCountry == "" {
		return errors.New("certification_country is required when a certification filter is set")
	}

	if p.WithWatchProviders != "" && p.WatchRegion == "" {
		return errors.New("watch_region is required when with_watch_providers is set")
	}

	return nil
}

// Validate checks cross-field rules that a single JSON field can't express
// on its own. Intended to run once, at catalog create/update time in the
// Builder API, before params are persisted — downstream consumers (Media
// adapter, addon server) trust that anything in catalogs.params already
// passed this check.
func (p TMDBMovieParams) Validate() error {
	if p.SortBy != "" && !validMovieSortValues[p.SortBy] {
		return fmt.Errorf("invalid sort_by %q for movie catalog", p.SortBy)
	}

	hasFixedRange := p.PrimaryReleaseDateGte != "" || p.PrimaryReleaseDateLte != ""
	if hasFixedRange && p.ReleasedWithinDays != 0 {
		return errors.New("cannot set both a fixed release date range and released_within_days")
	}

	return p.TMDBCommonParams.validate()
}

// Validate checks cross-field rules that a single JSON field can't express
// on its own. See TMDBMovieParams.Validate for where this runs.
func (p TMDBTVParams) Validate() error {
	if p.SortBy != "" && !validTVSortValues[p.SortBy] {
		return fmt.Errorf("invalid sort_by %q for tv catalog", p.SortBy)
	}

	hasFixedRange := p.FirstAirDateGte != "" || p.FirstAirDateLte != ""
	if hasFixedRange && p.AiredWithinDays != 0 {
		return errors.New("cannot set both a fixed first_air_date range and aired_within_days")
	}

	return p.TMDBCommonParams.validate()
}
