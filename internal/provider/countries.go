package provider

import (
	"context"

	"github.com/hiidz/uno/internal/jsonwire"
)

// Countries returns TMDB's full ISO 3166-1 country table, for naming the
// country codes the certification list returns. Cached for the process's
// lifetime and, like Languages, not split by catalogType, so the whole
// table lives under one empty cache key.
func (c *TMDBClient) Countries(ctx context.Context) ([]Country, error) {
	return c.countries.load("", func() ([]Country, error) {
		var out []Country
		if err := c.get(ctx, "/configuration/countries", nil, &out); err != nil {
			return nil, err
		}
		return jsonwire.OrEmpty(out), nil
	})
}
