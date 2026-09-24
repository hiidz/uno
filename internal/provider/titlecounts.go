package provider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// The company and network searches each turn a name match into a short list
// a user can pick from: count the titles discover credits to each match,
// drop the near-empty duplicates, and order by count.
const (
	// maxCountedMatches is how many name matches a search counts titles for
	// and can return.
	maxCountedMatches = 10

	// minMatchTitles is the fewest titles a search result needs to be
	// returned. Both TMDB vocabularies list same-named duplicates holding zero
	// or one title alongside, and sometimes ahead of, the entry a user means.
	minMatchTitles = 5

	// titleCountsTTL bounds how long a title count is served: a studio's or
	// network's count moves as it releases titles.
	titleCountsTTL = 24 * time.Hour
)

// titleCount returns how many titles catalogType's discover credits to id
// under the discover filter param (with_companies, with_networks), memoized
// per param, catalog type and id. A TMDB 404 is reported without
// ErrNotFound, since the search it serves has found nothing missing.
func (c *TMDBClient) titleCount(ctx context.Context, param, catalogType string, id int) (int, error) {
	endpoint, err := catalogEndpoint(catalogType)
	if err != nil {
		return 0, err
	}
	return c.titleCounts.load(param+":"+catalogType+":"+strconv.Itoa(id), func() (int, error) {
		resp, err := c.discover(ctx, endpoint, url.Values{param: {strconv.Itoa(id)}})
		if errors.Is(err, ErrNotFound) {
			return 0, fmt.Errorf("provider: count titles for %s=%d: TMDB answered 404 for %s", param, id, endpoint)
		}
		if err != nil {
			return 0, fmt.Errorf("provider: count titles for %s=%d: %w", param, id, err)
		}
		return resp.TotalResults, nil
	})
}

// forEachMatch calls fn for each index below n, with the same concurrency
// bound as resolveMetas. The first error cancels the context the calls still
// running were given, and is returned once all of them have finished.
func forEachMatch(ctx context.Context, n int, fn func(ctx context.Context, i int) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, externalIDsConcurrency)
	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
	)
	for i := range n {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := fn(ctx, i); err != nil {
				once.Do(func() {
					firstErr = err
					cancel()
				})
			}
		})
	}
	wg.Wait()
	return firstErr
}
