package provider

import "context"

// Languages returns TMDB's full ISO 639-1 language table. Unlike Genres and
// Certifications, it isn't split by catalogType — TMDB's language list is
// the same set regardless of movie vs tv, since it backs
// with_original_language on both.
func (c *TMDBClient) Languages(ctx context.Context) ([]Language, error) {
	var out []Language
	if err := c.get(ctx, "/configuration/languages", nil, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Language{}
	}
	return out, nil
}
