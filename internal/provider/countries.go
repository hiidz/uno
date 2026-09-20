package provider

import (
	"context"

	"github.com/hiidz/uno/internal/jsonwire"
)

// Countries returns TMDB's full ISO 3166-1 country table, for naming the
// country codes the certification list returns. Not split by catalogType —
// TMDB's country list is the same set regardless of movie vs tv.
func (c *TMDBClient) Countries(ctx context.Context) ([]Country, error) {
	var out []Country
	if err := c.get(ctx, "/configuration/countries", nil, &out); err != nil {
		return nil, err
	}
	return jsonwire.OrEmpty(out), nil
}
