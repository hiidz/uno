package provider

import (
	"cmp"
	"context"
	"slices"

	"github.com/hiidz/uno/internal/jsonwire"
)

// Company is one TMDB production company — with_companies takes ID, and the
// picker shows Name.
type Company struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// CompanyMatch is one company search result: a Company plus what the picker
// needs to tell same-named companies apart. OriginCountry may be "".
// TitleCount is how many titles of the searched catalog type TMDB's discover
// credits to the company.
type CompanyMatch struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	OriginCountry string `json:"origin_country"`
	TitleCount    int    `json:"title_count"`
}

// Company returns the TMDB company with id, memoized. It serves both recipe
// validation's existence check and the picker's id-to-name resolution. An id
// TMDB doesn't have wraps ErrNotFound.
func (c *TMDBClient) Company(ctx context.Context, id int) (Company, error) {
	return entityByID(ctx, c, c.companies, "/company/%d", id)
}

// SearchCompanies returns the companies on the first page of TMDB's company
// search for query that credit at least minMatchTitles titles of catalogType,
// most titles first (ties keep TMDB's order). Only the first
// maxCountedMatches results are counted, each by titleCount; the search
// itself is not cached, since results vary with every keystroke the picker
// sends.
//
// An unknown catalogType or a blank query is rejected before TMDB is
// contacted. A failed count fails the whole search rather than dropping the
// result it was for.
func (c *TMDBClient) SearchCompanies(ctx context.Context, catalogType, query string) ([]CompanyMatch, error) {
	if _, err := catalogEndpoint(catalogType); err != nil {
		return nil, err
	}
	matches, err := searchEntities[CompanyMatch](ctx, c, "/search/company", query)
	if err != nil {
		return nil, err
	}
	matches = matches[:min(len(matches), maxCountedMatches)]

	err = forEachMatch(ctx, len(matches), func(ctx context.Context, i int) error {
		n, err := c.titleCount(ctx, "with_companies", catalogType, matches[i].ID)
		matches[i].TitleCount = n
		return err
	})
	if err != nil {
		return nil, err
	}

	matches = slices.DeleteFunc(matches, func(m CompanyMatch) bool { return m.TitleCount < minMatchTitles })
	slices.SortStableFunc(matches, func(a, b CompanyMatch) int { return cmp.Compare(b.TitleCount, a.TitleCount) })
	return jsonwire.OrEmpty(matches), nil
}
