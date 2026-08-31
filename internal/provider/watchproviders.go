package provider

import (
	"context"
	"fmt"
	"net/url"
	"sort"
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
// Netflix somewhere in the middle of it.
func (c *TMDBClient) WatchProviders(ctx context.Context, catalogType, region string) ([]WatchProvider, error) {
	kind, ok := externalIDsMediaType[catalogType]
	if !ok {
		return nil, fmt.Errorf("%w: got %q", ErrInvalidCatalogType, catalogType)
	}

	query := url.Values{}
	if region != "" {
		query.Set("watch_region", region)
	}

	var out watchProviderListResponse
	if err := c.get(ctx, fmt.Sprintf("/watch/providers/%s", kind), query, &out); err != nil {
		return nil, err
	}
	if out.Results == nil {
		out.Results = []WatchProvider{}
	}

	sort.SliceStable(out.Results, func(i, j int) bool {
		a, b := out.Results[i], out.Results[j]
		if a.DisplayPriority != b.DisplayPriority {
			return a.DisplayPriority < b.DisplayPriority
		}
		return a.ProviderName < b.ProviderName
	})
	return out.Results, nil
}

// WatchRegions returns the countries TMDB has watch-provider data for, which
// is what watch_region accepts. Not the same set as Countries — that table is
// every ISO 3166-1 country, and most of them have no provider data at all.
func (c *TMDBClient) WatchRegions(ctx context.Context) ([]WatchRegion, error) {
	var out watchRegionListResponse
	if err := c.get(ctx, "/watch/providers/regions", nil, &out); err != nil {
		return nil, err
	}
	if out.Results == nil {
		out.Results = []WatchRegion{}
	}
	return out.Results, nil
}
