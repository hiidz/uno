package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// ErrInvalidParams marks a recipe whose TMDB-vocabulary fields name
// something TMDB's own lists don't contain. It exists so callers can tell a
// rejected recipe (the client's fault) from TMDB being unreachable while
// checking one (ours) — [TMDBClient.ValidateParams] returns the latter
// unwrapped.
var ErrInvalidParams = errors.New("invalid params")

// ValidateParams checks the recipe fields that carry TMDB's own vocabulary
// — genre ids, a language code, a watch region and its provider ids, a
// certification country and its scale — against the lists TMDB publishes
// for them. It complements [CatalogParams.Validate], which covers the rules
// that need no network (enums, bounds, required-together pairs) and runs
// first.
//
// Each list is fetched only when the field that needs it is set, so a
// recipe using none of them costs nothing. Every list is memoized on the
// client (see internal/provider/cache.go), so a set field costs one TMDB
// round trip on a cold cache and none after that.
//
// A rejected value is wrapped in ErrInvalidParams. A failure to reach TMDB
// is returned as-is.
func (c *TMDBClient) ValidateParams(ctx context.Context, catalogType, paramsJSON string) error {
	if _, err := paramsFor(catalogType); err != nil {
		return err
	}

	// Both recipe types embed TMDBCommonParams, and every field checked here
	// lives on it, so decoding straight into it covers movie and tv alike.
	var p TMDBCommonParams
	if err := json.Unmarshal([]byte(paramsJSON), &p); err != nil {
		return fmt.Errorf("%w: decode %s params: %w", ErrInvalidParams, catalogType, err)
	}

	if p.WithGenres != "" || p.WithoutGenres != "" {
		genres, err := c.Genres(ctx, catalogType)
		if err != nil {
			return err
		}
		allowed := make(map[int]bool, len(genres))
		for _, g := range genres {
			allowed[g.ID] = true
		}
		if err := checkIDList("with_genres", p.WithGenres, allowed, "genre"); err != nil {
			return err
		}
		if err := checkIDList("without_genres", p.WithoutGenres, allowed, "genre"); err != nil {
			return err
		}
	}

	if p.WithOriginalLanguage != "" {
		languages, err := c.Languages(ctx)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(languages, func(l Language) bool { return l.ISO6391 == p.WithOriginalLanguage }) {
			return fmt.Errorf("%w: with_original_language %q is not a TMDB language code", ErrInvalidParams, p.WithOriginalLanguage)
		}
	}

	if p.WatchRegion != "" {
		regions, err := c.WatchRegions(ctx)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(regions, func(r WatchRegion) bool { return r.ISO31661 == p.WatchRegion }) {
			return fmt.Errorf("%w: watch_region %q is not a TMDB watch region", ErrInvalidParams, p.WatchRegion)
		}
	}

	if p.WithWatchProviders != "" {
		// validate() already requires WatchRegion alongside this field, so
		// the list is scoped to the region the recipe actually queries —
		// a provider id is only meaningful within one.
		providers, err := c.WatchProviders(ctx, catalogType, p.WatchRegion)
		if err != nil {
			return err
		}
		allowed := make(map[int]bool, len(providers))
		for _, wp := range providers {
			allowed[wp.ProviderID] = true
		}
		if err := checkIDList("with_watch_providers", p.WithWatchProviders, allowed, "watch provider"); err != nil {
			return err
		}
	}

	return c.validateCertifications(ctx, catalogType, p)
}

// validateCertifications checks the certification country against TMDB's
// list of rating systems and each certification value against that
// country's own scale — the scales differ per country, so the country has
// to resolve before the values can be checked at all.
func (c *TMDBClient) validateCertifications(ctx context.Context, catalogType string, p TMDBCommonParams) error {
	if p.CertificationCountry == "" && p.Certification == "" && p.CertificationGte == "" && p.CertificationLte == "" {
		return nil
	}

	byCountry, err := c.Certifications(ctx, catalogType)
	if err != nil {
		return err
	}
	scale, ok := byCountry[p.CertificationCountry]
	if !ok {
		return fmt.Errorf("%w: certification_country %q has no TMDB certification scale", ErrInvalidParams, p.CertificationCountry)
	}

	allowed := make(map[string]bool, len(scale))
	for _, cert := range scale {
		allowed[cert.Certification] = true
	}
	for _, f := range []struct{ name, value string }{
		{"certification", p.Certification},
		{"certification_gte", p.CertificationGte},
		{"certification_lte", p.CertificationLte},
	} {
		if f.value != "" && !allowed[f.value] {
			return fmt.Errorf("%w: %s %q is not in %s's certification scale", ErrInvalidParams, f.name, f.value, p.CertificationCountry)
		}
	}
	return nil
}

// checkIDList walks a comma (AND) or pipe (OR) separated TMDB id list and
// reports the first entry that isn't an integer or isn't in allowed. label
// names what the ids are, for the error text.
func checkIDList(field, list string, allowed map[int]bool, label string) error {
	for _, part := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == '|' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.Atoi(part)
		if err != nil {
			return fmt.Errorf("%w: %s entry %q is not a numeric id", ErrInvalidParams, field, part)
		}
		if !allowed[id] {
			return fmt.Errorf("%w: %s entry %q is not a TMDB %s id", ErrInvalidParams, field, part, label)
		}
	}
	return nil
}
