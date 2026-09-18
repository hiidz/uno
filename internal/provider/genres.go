package provider

import (
	"context"
	"fmt"
	"slices"
)

// Genres returns TMDB's genre list for one catalog type, cached in memory
// for the process's lifetime once fetched: the list is effectively static,
// and the addon manifest reads it on every request. A failed fetch is not
// cached, so the next call retries.
//
// catalogType is Uno/Stremio's vocabulary ("movie"/"series"), matching
// every other route — not TMDB's ("movie"/"tv"). Translated via
// externalIDsMediaType, the same map catalog.go's resolveMetas uses for
// this exact split.
func (c *TMDBClient) Genres(ctx context.Context, catalogType string) ([]Genre, error) {
	kind, ok := externalIDsMediaType[catalogType]
	if !ok {
		return nil, fmt.Errorf("%w: got %q", ErrInvalidCatalogType, catalogType)
	}

	c.genresMu.RLock()
	cached, ok := c.genresCache[kind]
	c.genresMu.RUnlock()
	if ok {
		return slices.Clone(cached), nil
	}

	var out genreListResponse
	if err := c.get(ctx, fmt.Sprintf("/genre/%s/list", kind), nil, &out); err != nil {
		return nil, err
	}
	// Defensive, like the vault package's orEmpty: TMDB is expected to
	// always return a genres array, but a nil slice here would round-trip
	// as `null` over the wire.
	if out.Genres == nil {
		out.Genres = []Genre{}
	}

	c.genresMu.Lock()
	c.genresCache[kind] = out.Genres
	c.genresMu.Unlock()
	return slices.Clone(out.Genres), nil
}
