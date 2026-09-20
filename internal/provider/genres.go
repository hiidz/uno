package provider

import (
	"context"
	"fmt"

	"github.com/hiidz/uno/internal/jsonwire"
)

// Genres returns TMDB's genre list for one catalog type, cached for the
// process's lifetime once fetched: the list is effectively static, and the
// addon manifest reads it on every request.
//
// catalogType is Uno/Stremio's vocabulary ("movie"/"series"), matching
// every other route — not TMDB's ("movie"/"tv"). Translated via
// externalIDsMediaType, the same map catalog.go's resolveMetas uses for
// this exact split, and the translated kind is the cache key.
func (c *TMDBClient) Genres(ctx context.Context, catalogType string) ([]Genre, error) {
	kind, ok := externalIDsMediaType[catalogType]
	if !ok {
		return nil, fmt.Errorf("%w: got %q", ErrInvalidCatalogType, catalogType)
	}

	return c.genres.load(kind, func() ([]Genre, error) {
		var out genreListResponse
		if err := c.get(ctx, fmt.Sprintf("/genre/%s/list", kind), nil, &out); err != nil {
			return nil, err
		}
		// Defensive: TMDB is expected to always return a genres array, but a
		// nil slice here would round-trip as `null` over the wire.
		return jsonwire.OrEmpty(out.Genres), nil
	})
}
