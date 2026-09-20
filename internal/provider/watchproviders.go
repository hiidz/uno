package provider

import (
	"cmp"
	"context"
	"fmt"
	"net/url"
	"slices"

	"github.com/hiidz/uno/internal/jsonwire"
)

// WatchProviders returns the streaming services TMDB can filter on in one
// region, for one catalog type. The list is region-scoped because a service
// carries a different id per market and TMDB only reports the ones it has
// data for.
//
// catalogType is Uno/Stremio's vocabulary ("movie"/"series"), matching every
// other route, and is translated the same way Genres translates it.
//
// Sorted by TMDB's own DisplayPriority, then by name: the picker shows this
// list as-is, and 291 services in TMDB's arbitrary response order puts
// Netflix somewhere in the middle of it. The sort runs before the result is
// cached, so it is paid once per region rather than per request.
//
// Cached under the catalog type and region, and unlike the lists beside it
// only for watchProviderTTL — see there. A region that isn't shaped like an
// ISO 3166-1 code is fetched without touching the cache, so the free-form
// region query param on the picker route can't grow the key space one entry
// per request.
func (c *TMDBClient) WatchProviders(ctx context.Context, catalogType, region string) ([]WatchProvider, error) {
	kind, ok := tmdbMediaType[catalogType]
	if !ok {
		return nil, fmt.Errorf("%w: got %q", ErrInvalidCatalogType, catalogType)
	}

	fetch := func() ([]WatchProvider, error) {
		query := url.Values{}
		if region != "" {
			query.Set("watch_region", region)
		}

		var out watchProviderListResponse
		if err := c.get(ctx, fmt.Sprintf("/watch/providers/%s", kind), query, &out); err != nil {
			return nil, err
		}
		results := jsonwire.OrEmpty(out.Results)

		slices.SortStableFunc(results, func(a, b WatchProvider) int {
			if a.DisplayPriority != b.DisplayPriority {
				return cmp.Compare(a.DisplayPriority, b.DisplayPriority)
			}
			return cmp.Compare(a.ProviderName, b.ProviderName)
		})
		return results, nil
	}

	if !cacheableRegion(region) {
		return fetch()
	}
	return c.watchProviders.load(kind+"/"+region, fetch)
}

// cacheableRegion reports whether region is shaped like the ISO 3166-1
// codes WatchRegions returns — empty, or two uppercase letters. Recipe
// validation checks a recipe's watch_region against that list before it
// ever asks for providers, so the save path always qualifies; only the
// picker route, which takes the region straight off the query string, can
// reach here with something else.
func cacheableRegion(region string) bool {
	if region == "" {
		return true
	}
	if len(region) != 2 {
		return false
	}
	for i := range len(region) {
		if region[i] < 'A' || region[i] > 'Z' {
			return false
		}
	}
	return true
}

// WatchRegions returns the countries TMDB has watch-provider data for, which
// is what watch_region accepts, cached for the process's lifetime under one
// empty key. Not the same set as Countries — that table is every ISO 3166-1
// country, and most of them have no provider data at all.
func (c *TMDBClient) WatchRegions(ctx context.Context) ([]WatchRegion, error) {
	return c.watchRegions.load("", func() ([]WatchRegion, error) {
		var out watchRegionListResponse
		if err := c.get(ctx, "/watch/providers/regions", nil, &out); err != nil {
			return nil, err
		}
		return jsonwire.OrEmpty(out.Results), nil
	})
}
