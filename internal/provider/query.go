package provider

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// buildDiscoverQuery decodes the stored recipe and translates it into TMDB's
// literal query params. Done field-by-field rather than a generic
// json-roundtrip dump: our own storage tags are underscore-only
// (vote_average_gte) but TMDB's real range params use a dot
// (vote_average.gte) — dumping the struct directly would silently produce
// query keys TMDB doesn't recognize and the filter would just never apply.
func buildDiscoverQuery(catalogType, paramsJSON string) (query url.Values, randomized bool, err error) {
	switch catalogType {
	case "movie":
		var p TMDBMovieParams
		if err := json.Unmarshal([]byte(paramsJSON), &p); err != nil {
			return nil, false, fmt.Errorf("provider: decode movie params: %w", err)
		}
		return movieQuery(p), p.Randomized, nil
	case "series":
		var p TMDBTVParams
		if err := json.Unmarshal([]byte(paramsJSON), &p); err != nil {
			return nil, false, fmt.Errorf("provider: decode tv params: %w", err)
		}
		return tvQuery(p), p.Randomized, nil
	default:
		return nil, false, fmt.Errorf("%w: got %q", ErrInvalidCatalogType, catalogType)
	}
}

func commonQuery(p TMDBCommonParams) url.Values {
	q := url.Values{}
	setIf(q, "sort_by", p.SortBy)
	setIf(q, "with_genres", p.WithGenres)
	setIf(q, "without_genres", p.WithoutGenres)
	setIf(q, "with_original_language", p.WithOriginalLanguage)
	setIf(q, "with_watch_providers", p.WithWatchProviders)
	setIf(q, "watch_region", p.WatchRegion)
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

func movieQuery(p TMDBMovieParams) url.Values {
	q := commonQuery(p.TMDBCommonParams)
	setIf(q, "primary_release_date.gte", p.PrimaryReleaseDateGte)
	setIf(q, "primary_release_date.lte", p.PrimaryReleaseDateLte)
	if p.ReleasedWithinDays > 0 {
		q.Set("primary_release_date.gte", daysAgo(p.ReleasedWithinDays))
	}
	return q
}

func tvQuery(p TMDBTVParams) url.Values {
	q := commonQuery(p.TMDBCommonParams)
	setIf(q, "first_air_date.gte", p.FirstAirDateGte)
	setIf(q, "first_air_date.lte", p.FirstAirDateLte)
	if p.AiredWithinDays > 0 {
		q.Set("air_date.gte", daysAgo(p.AiredWithinDays))
	}
	return q
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
