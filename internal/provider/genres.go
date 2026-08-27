package provider

import (
	"context"
	"fmt"
)

// TODO: add an in-memory cache — genre lists barely change, and every call
// is a free round trip against TMDB's rate limit for something effectively
// static.
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
	return out.Genres, nil
}
