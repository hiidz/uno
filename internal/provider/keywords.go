package provider

import "context"

// Keyword is one TMDB keyword — with_keywords takes ID, and the picker shows
// Name.
type Keyword struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Keyword returns the TMDB keyword with id, memoized. It serves both recipe validation's existence check and the
// picker's id-to-name resolution. An id TMDB doesn't have wraps ErrNotFound.
func (c *TMDBClient) Keyword(ctx context.Context, id int) (Keyword, error) {
	return entityByID(ctx, c, c.keywords, "/keyword/%d", id)
}

// SearchKeywords returns the first page of TMDB's keyword search for query.
// Not cached: results vary with every keystroke the picker sends.
func (c *TMDBClient) SearchKeywords(ctx context.Context, query string) ([]Keyword, error) {
	return searchEntities[Keyword](ctx, c, "/search/keyword", query)
}
