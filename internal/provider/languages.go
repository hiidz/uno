package provider

import (
	"context"

	"github.com/hiidz/uno/internal/jsonwire"
)

// Languages returns TMDB's full ISO 639-1 language table, cached for the
// process's lifetime. Unlike Genres and Certifications it isn't split by
// catalogType — TMDB's language list is the same set regardless of movie vs
// tv, since it backs with_original_language on both — so the whole table
// lives under one empty cache key.
func (c *TMDBClient) Languages(ctx context.Context) ([]Language, error) {
	return c.languages.load("", func() ([]Language, error) {
		var out []Language
		if err := c.get(ctx, "/configuration/languages", nil, &out); err != nil {
			return nil, err
		}
		return jsonwire.OrEmpty(out), nil
	})
}
