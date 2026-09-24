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
// for them, and company, keyword, collection and network ids against TMDB
// one id at a time, since its API serves no whole list of any of them. It complements
// [CatalogParams.Validate], which covers the rules that need no network
// (enums, bounds, required-together pairs) and runs first.
//
// Each list is fetched only when the field that needs it is set, so a
// recipe using none of them costs nothing. Every list and every looked-up
// id is memoized on the client (see internal/provider/cache.go), so a set
// field costs TMDB round trips on a cold cache and none after that.
//
// A rejected value is wrapped in ErrInvalidParams. A failure to reach TMDB
// is returned as-is.
func (c *TMDBClient) ValidateParams(ctx context.Context, catalogType, paramsJSON string) error {
	if _, err := paramsFor(catalogType); err != nil {
		return err
	}

	var p vocabParams
	if err := json.Unmarshal([]byte(paramsJSON), &p); err != nil {
		return fmt.Errorf("%w: decode %s params: %w", ErrInvalidParams, catalogType, err)
	}

	for _, check := range []func(context.Context, string, vocabParams) error{
		c.validateGenres,
		c.validateLanguage,
		c.validateWatchRegion,
		c.validateWatchProviders,
		c.validateCompaniesAndKeywords,
		c.validateCollection,
		c.validateNetworks,
		c.validateCertifications,
	} {
		if err := check(ctx, catalogType, p); err != nil {
			return err
		}
	}
	return nil
}

// vocabParams is the slice of a recipe ValidateParams checks. Both recipe
// types embed TMDBCommonParams, and every field checked here but
// with_collection and with_networks lives on it, so decoding into it covers
// movie and tv alike. Those two are decoded alongside for either type, so a
// recipe of the other type carrying one is rejected rather than silently
// ignored.
type vocabParams struct {
	TMDBCommonParams
	WithCollection string `json:"with_collection"`
	WithNetworks   string `json:"with_networks"`
}

// validateGenres checks with_genres and without_genres against TMDB's genre
// list for the catalog type.
func (c *TMDBClient) validateGenres(ctx context.Context, catalogType string, p vocabParams) error {
	if p.WithGenres == "" && p.WithoutGenres == "" {
		return nil
	}
	genres, err := c.Genres(ctx, catalogType)
	if err != nil {
		return err
	}
	allowed := idSet(genres, func(g Genre) int { return g.ID })
	if err := checkIDList("with_genres", p.WithGenres, allowed, "genre"); err != nil {
		return err
	}
	return checkIDList("without_genres", p.WithoutGenres, allowed, "genre")
}

// validateLanguage checks with_original_language against TMDB's language
// codes.
func (c *TMDBClient) validateLanguage(ctx context.Context, _ string, p vocabParams) error {
	if p.WithOriginalLanguage == "" {
		return nil
	}
	languages, err := c.Languages(ctx)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(languages, func(l Language) bool { return l.ISO6391 == p.WithOriginalLanguage }) {
		return fmt.Errorf("%w: with_original_language %q is not a TMDB language code", ErrInvalidParams, p.WithOriginalLanguage)
	}
	return nil
}

// validateWatchRegion checks watch_region against TMDB's watch regions.
func (c *TMDBClient) validateWatchRegion(ctx context.Context, _ string, p vocabParams) error {
	if p.WatchRegion == "" {
		return nil
	}
	regions, err := c.WatchRegions(ctx)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(regions, func(r WatchRegion) bool { return r.ISO31661 == p.WatchRegion }) {
		return fmt.Errorf("%w: watch_region %q is not a TMDB watch region", ErrInvalidParams, p.WatchRegion)
	}
	return nil
}

// validateWatchProviders checks with_watch_providers against TMDB's
// providers for the recipe's watch_region. validate() already requires
// WatchRegion alongside this field, so the list is scoped to the region the
// recipe actually queries — a provider id is only meaningful within one.
func (c *TMDBClient) validateWatchProviders(ctx context.Context, catalogType string, p vocabParams) error {
	if p.WithWatchProviders == "" {
		return nil
	}
	providers, err := c.WatchProviders(ctx, catalogType, p.WatchRegion)
	if err != nil {
		return err
	}
	allowed := idSet(providers, func(wp WatchProvider) int { return wp.ProviderID })
	return checkIDList("with_watch_providers", p.WithWatchProviders, allowed, "watch provider")
}

// validateCompaniesAndKeywords looks up each include and exclude company
// and keyword id on TMDB.
func (c *TMDBClient) validateCompaniesAndKeywords(ctx context.Context, _ string, p vocabParams) error {
	if err := checkEntityIDs(ctx, "with_companies", p.WithCompanies, "company", c.Company); err != nil {
		return err
	}
	if err := checkEntityIDs(ctx, "with_keywords", p.WithKeywords, "keyword", c.Keyword); err != nil {
		return err
	}
	if err := checkEntityIDs(ctx, "without_companies", p.WithoutCompanies, "company", c.Company); err != nil {
		return err
	}
	return checkEntityIDs(ctx, "without_keywords", p.WithoutKeywords, "keyword", c.Keyword)
}

// validateCollection rejects with_collection on anything but a movie
// catalog, then looks its id up on TMDB.
func (c *TMDBClient) validateCollection(ctx context.Context, catalogType string, p vocabParams) error {
	if p.WithCollection == "" {
		return nil
	}
	if catalogType != "movie" {
		return fmt.Errorf("%w: with_collection applies to movie catalogs only", ErrInvalidParams)
	}
	return checkEntityIDs(ctx, "with_collection", p.WithCollection, "collection", c.Collection)
}

// validateNetworks rejects with_networks on anything but a series catalog,
// then looks each id up on TMDB.
func (c *TMDBClient) validateNetworks(ctx context.Context, catalogType string, p vocabParams) error {
	if p.WithNetworks == "" {
		return nil
	}
	if catalogType != "series" {
		return fmt.Errorf("%w: with_networks applies to series catalogs only", ErrInvalidParams)
	}
	return checkEntityIDs(ctx, "with_networks", p.WithNetworks, "network", c.Network)
}

// certField is one certification value of a recipe, named for error text.
type certField struct{ name, value string }

// validateCertifications checks the certification country against TMDB's
// list of rating systems and each certification value against that
// country's own scale — the scales differ per country, so the country has
// to resolve before the values can be checked at all.
func (c *TMDBClient) validateCertifications(ctx context.Context, catalogType string, p vocabParams) error {
	fields := []certField{
		{"certification", p.Certification},
		{"certification_gte", p.CertificationGte},
		{"certification_lte", p.CertificationLte},
	}
	if p.CertificationCountry == "" && !slices.ContainsFunc(fields, func(f certField) bool { return f.value != "" }) {
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
	return checkCertifications(p.CertificationCountry, scale, fields)
}

// checkCertifications reports the first set field whose value isn't on
// country's certification scale.
func checkCertifications(country string, scale []Certification, fields []certField) error {
	allowed := make(map[string]bool, len(scale))
	for _, cert := range scale {
		allowed[cert.Certification] = true
	}
	for _, f := range fields {
		if f.value != "" && !allowed[f.value] {
			return fmt.Errorf("%w: %s %q is not in %s's certification scale", ErrInvalidParams, f.name, f.value, country)
		}
	}
	return nil
}

// idSet collects the TMDB id of each item, for checkIDList.
func idSet[T any](items []T, id func(T) int) map[int]bool {
	set := make(map[int]bool, len(items))
	for _, item := range items {
		set[id(item)] = true
	}
	return set
}

// checkIDList reports the first entry of a TMDB id list that isn't an
// integer or isn't in allowed. label names what the ids are, for the error
// text.
func checkIDList(field, list string, allowed map[int]bool, label string) error {
	ids, err := parseIDList(field, list)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if !allowed[id] {
			return fmt.Errorf("%w: %s entry \"%d\" is not a TMDB %s id", ErrInvalidParams, field, id, label)
		}
	}
	return nil
}

// checkEntityIDs is checkIDList for the vocabularies TMDB publishes no whole
// list of: each id is looked up on its own, and one TMDB doesn't have
// (ErrNotFound) is rejected with ErrInvalidParams. Any other lookup failure
// is returned as-is.
func checkEntityIDs[T any](ctx context.Context, field, list, label string, lookup func(context.Context, int) (T, error)) error {
	ids, err := parseIDList(field, list)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := lookup(ctx, id); err != nil {
			if errors.Is(err, ErrNotFound) {
				return fmt.Errorf("%w: %s entry \"%d\" is not a TMDB %s id", ErrInvalidParams, field, id, label)
			}
			return err
		}
	}
	return nil
}

// parseIDList splits a comma (AND) or pipe (OR) separated TMDB id list into
// its ids, rejecting the first entry that isn't an integer.
func parseIDList(field, list string) ([]int, error) {
	var ids []int
	for _, part := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == '|' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("%w: %s entry %q is not a numeric id", ErrInvalidParams, field, part)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
