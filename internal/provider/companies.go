package provider

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"

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

const (
	// maxCompanyMatches is how many of TMDB's first search page SearchCompanies
	// counts titles for and can return.
	maxCompanyMatches = 10

	// minCompanyTitles is the fewest titles a company search result needs to
	// be returned. TMDB's search lists same-named duplicates holding zero or
	// one title alongside, and sometimes ahead of, the studio a user means.
	minCompanyTitles = 5

	// companyTitlesTTL bounds how long a company's title count is served: a
	// studio's count moves as it releases titles.
	companyTitlesTTL = 24 * time.Hour
)

// Company returns the TMDB company with id, memoized. It serves both recipe
// validation's existence check and the picker's id-to-name resolution. An id
// TMDB doesn't have wraps ErrNotFound.
func (c *TMDBClient) Company(ctx context.Context, id int) (Company, error) {
	return entityByID(ctx, c, c.companies, "/company/%d", id)
}

// SearchCompanies returns the companies on the first page of TMDB's company
// search for query that credit at least minCompanyTitles titles of
// catalogType, most titles first (ties keep TMDB's order). Only the first
// maxCompanyMatches results are counted. Each count is one discover call,
// run with the same concurrency bound as resolveMetas and memoized for
// companyTitlesTTL; the search itself is not cached, since results vary with
// every keystroke the picker sends.
//
// An unknown catalogType or a blank query is rejected before TMDB is
// contacted. A failed count fails the whole search rather than dropping the
// result it was for.
func (c *TMDBClient) SearchCompanies(ctx context.Context, catalogType, query string) ([]CompanyMatch, error) {
	endpoint, err := catalogEndpoint(catalogType)
	if err != nil {
		return nil, err
	}
	matches, err := searchEntities[CompanyMatch](ctx, c, "/search/company", query)
	if err != nil {
		return nil, err
	}
	matches = matches[:min(len(matches), maxCompanyMatches)]

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, externalIDsConcurrency)
	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
	)
	for i := range matches {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			n, err := c.companyTitleCount(ctx, catalogType, endpoint, matches[i].ID)
			if err != nil {
				once.Do(func() {
					firstErr = err
					cancel()
				})
				return
			}
			matches[i].TitleCount = n
		})
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	matches = slices.DeleteFunc(matches, func(m CompanyMatch) bool { return m.TitleCount < minCompanyTitles })
	slices.SortStableFunc(matches, func(a, b CompanyMatch) int { return cmp.Compare(b.TitleCount, a.TitleCount) })
	return jsonwire.OrEmpty(matches), nil
}

// companyTitleCount returns how many titles endpoint's discover credits to
// company id, memoized per catalog type and id. A TMDB 404 is reported
// without ErrNotFound, since the company search it serves has found nothing
// missing.
func (c *TMDBClient) companyTitleCount(ctx context.Context, catalogType, endpoint string, id int) (int, error) {
	return c.companyTitles.load(catalogType+":"+strconv.Itoa(id), func() (int, error) {
		resp, err := c.discover(ctx, endpoint, url.Values{"with_companies": {strconv.Itoa(id)}})
		if errors.Is(err, ErrNotFound) {
			return 0, fmt.Errorf("provider: count titles for company %d: TMDB answered 404 for %s", id, endpoint)
		}
		if err != nil {
			return 0, fmt.Errorf("provider: count titles for company %d: %w", id, err)
		}
		return resp.TotalResults, nil
	})
}
