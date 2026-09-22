package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
)

// CatalogParams is a decoded catalog recipe: everything the rest of the
// package needs from one, whichever catalog type it came from.
// TMDBMovieParams and TMDBTVParams are the implementations.
//
// This interface plus paramsFor is what keeps the movie/series split in one
// place. Validation, query building and fingerprinting each used to switch on
// catalogType and spell out both arms, so a third catalog type meant three
// parallel edits; now it means one case in paramsFor.
type CatalogParams interface {
	// Validate checks the cross-field rules a single JSON field can't express.
	Validate() error
	// DiscoverQuery translates the recipe into TMDB's literal query params.
	DiscoverQuery() url.Values
	// IsRandomized reports whether the recipe shuffles rather than serving
	// page 1 — see FetchCatalogPage.
	IsRandomized() bool
}

// paramsFor returns an empty params struct for catalogType. The pointer
// matters: it is what DecodeParams unmarshals into, and the value-receiver
// methods above are in a pointer's method set too.
func paramsFor(catalogType string) (CatalogParams, error) {
	switch catalogType {
	case "movie":
		return &TMDBMovieParams{}, nil
	case "series":
		return &TMDBTVParams{}, nil
	default:
		return nil, fmt.Errorf("%w: got %q", ErrInvalidCatalogType, catalogType)
	}
}

// DecodeParams decodes a stored recipe's params JSON as catalogType's own
// shape. The one place a catalog type becomes a concrete params struct.
func DecodeParams(catalogType, paramsJSON string) (CatalogParams, error) {
	p, err := paramsFor(catalogType)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(paramsJSON), p); err != nil {
		return nil, fmt.Errorf("provider: decode %s params: %w", catalogType, err)
	}
	return p, nil
}

// IsRandomized satisfies CatalogParams for every recipe type, since they all
// embed BaseParams.
func (p BaseParams) IsRandomized() bool { return p.Randomized }

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

// Language is one entry in TMDB's ISO 639-1 language table — the source for
// with_original_language's options, so the picker can't offer a code TMDB
// itself doesn't recognize.
type Language struct {
	ISO6391     string `json:"iso_639_1"`
	EnglishName string `json:"english_name"`
	Name        string `json:"name"`
}

// Country is one entry in TMDB's ISO 3166-1 country table — the source for
// display names of the country codes the certification list returns (that
// endpoint returns codes only, not names).
type Country struct {
	ISO31661    string `json:"iso_3166_1"`
	EnglishName string `json:"english_name"`
	NativeName  string `json:"native_name"`
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

	WithCompanies string `json:"with_companies,omitempty"` // comma (AND) or pipe (OR) separated TMDB company ids
	WithKeywords  string `json:"with_keywords,omitempty"`  // comma (AND) or pipe (OR) separated TMDB keyword ids

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

	// One TMDB collection id (e.g. 10, Star Wars). Set, it makes the recipe
	// a collection recipe: its titles are the collection's films rather than
	// a discover result (see collectionItems), and every other field but
	// Randomized must be unset — enforced in Validate.
	WithCollection string `json:"with_collection,omitempty"`
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

// tmdbMaxVoteAverage is the top of TMDB's rating scale; vote_average.gte
// and .lte are compared against it, so a value past it can only ever return
// nothing.
const tmdbMaxVoteAverage = 10.0

// maxWithinDays caps released_within_days / aired_within_days at the same 50
// years the builder's "Last N years" box allows, so a hand-crafted request
// can't exceed what the UI itself permits.
const maxWithinDays = 50 * 365

// maxEntityIDs caps how many ids with_companies and with_keywords each hold.
// ValidateParams looks every id up on TMDB one at a time, so the cap bounds
// the round trips one save can cost on a cold cache.
const maxEntityIDs = 20

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

	for _, f := range []struct{ name, list string }{
		{"with_companies", p.WithCompanies},
		{"with_keywords", p.WithKeywords},
	} {
		ids, err := parseIDList(f.name, f.list)
		if err != nil {
			return err
		}
		if len(ids) > maxEntityIDs {
			return fmt.Errorf("%s cannot hold more than %d ids", f.name, maxEntityIDs)
		}
	}

	// Zero means "unset" for every numeric field below (see setIntIf and
	// setFloatIf in query.go), so these bound the values that do get sent
	// rather than requiring one.
	for _, f := range []struct {
		name  string
		value float64
	}{
		{"vote_average_gte", p.VoteAverageGte},
		{"vote_average_lte", p.VoteAverageLte},
	} {
		if f.value < 0 || f.value > tmdbMaxVoteAverage {
			return fmt.Errorf("%s must be between 0 and %g", f.name, tmdbMaxVoteAverage)
		}
	}

	for _, f := range []struct {
		name  string
		value int
	}{
		{"vote_count_gte", p.VoteCountGte},
		{"vote_count_lte", p.VoteCountLte},
		{"with_runtime_gte", p.WithRuntimeGte},
		{"with_runtime_lte", p.WithRuntimeLte},
	} {
		if f.value < 0 {
			return fmt.Errorf("%s cannot be negative", f.name)
		}
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
	// DiscoverQuery's `> 0` guard would otherwise drop a negative silently,
	// leaving the recipe saved with a filter that never applies.
	if p.ReleasedWithinDays < 0 {
		return errors.New("released_within_days cannot be negative")
	}
	if p.ReleasedWithinDays > maxWithinDays {
		return fmt.Errorf("released_within_days cannot exceed %d (50 years)", maxWithinDays)
	}
	if strings.ContainsAny(p.WithCollection, ",|") {
		return errors.New("with_collection takes a single collection id")
	}
	if p.WithCollection != "" {
		rest := p
		rest.WithCollection, rest.Randomized = "", false
		if field := firstSetField(reflect.ValueOf(rest)); field != "" {
			return fmt.Errorf("%s cannot be combined with with_collection", field)
		}
	}

	return p.validate()
}

// firstSetField returns the JSON name of the first non-zero field of struct
// v, descending into embedded structs, or "" when every field is zero.
func firstSetField(v reflect.Value) string {
	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)
		if field.Anonymous {
			if name := firstSetField(v.Field(i)); name != "" {
				return name
			}
			continue
		}
		if !v.Field(i).IsZero() {
			if name, _, _ := strings.Cut(field.Tag.Get("json"), ","); name != "" && name != "-" {
				return name
			}
			return field.Name
		}
	}
	return ""
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
	// Same reason as TMDBMovieParams.Validate's released_within_days check.
	if p.AiredWithinDays < 0 {
		return errors.New("aired_within_days cannot be negative")
	}
	if p.AiredWithinDays > maxWithinDays {
		return fmt.Errorf("aired_within_days cannot exceed %d (50 years)", maxWithinDays)
	}

	return p.validate()
}

// WatchProvider is one streaming service TMDB can filter on —
// with_watch_providers takes ProviderID, and the picker shows Name.
// DisplayPriority is TMDB's own ranking of how prominent a service is in a
// given region, so a list sorted by it puts the household names first.
type WatchProvider struct {
	ProviderID      int    `json:"provider_id"`
	ProviderName    string `json:"provider_name"`
	DisplayPriority int    `json:"display_priority"`
	LogoPath        string `json:"logo_path"`
}

type watchProviderListResponse struct {
	Results []WatchProvider `json:"results"`
}

// WatchRegion is one country TMDB has watch-provider data for. A strict
// subset of the ISO 3166-1 country table, which is why watch_region reads
// from this rather than reusing Countries.
type WatchRegion struct {
	ISO31661    string `json:"iso_3166_1"`
	EnglishName string `json:"english_name"`
	NativeName  string `json:"native_name"`
}

type watchRegionListResponse struct {
	Results []WatchRegion `json:"results"`
}
