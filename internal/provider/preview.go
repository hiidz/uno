package provider

import (
	"context"
	"math/rand/v2"
	"strconv"
)

// PreviewItem is one tile in the builder's preview of a catalog recipe.
//
// Deliberately not [Meta]. Meta is Stremio's shape and its ID must be an IMDB
// id, which costs one /external_ids call per item — ~20 per page, on top of the
// single discover call. A preview needs poster, title and year, and all three
// are already on the discover response. So this type exists to make the cheap
// path expressible: previewing a catalog is one TMDB call, not twenty-one.
//
// TMDBID is here as a stable render key, nothing more. It is *not* an addon
// meta id and must never be used as one — that is exactly the confusion the
// IMDB resolution on the addon path exists to prevent.
type PreviewItem struct {
	TMDBID int    `json:"tmdb_id"`
	Title  string `json:"title"`
	Year   string `json:"year"`
	Poster string `json:"poster,omitempty"`
}

// PreviewCatalog runs a recipe against TMDB and returns one page of tiles
// plus TMDB's own count of how many titles match in total, saving nothing. It
// shares catalogEndpoint, buildDiscoverQuery and discover with
// [TMDBClient.FetchCatalogPage] — which is the point, since the
// underscore-to-dot param translation (vote_average_gte -> vote_average.gte)
// and the type->path mapping each have to live in exactly one place — but it
// deliberately skips resolveMetas.
//
// A plain recipe is page 1. A randomized recipe shuffles the way the addon path
// does, picking a random page within the first maxRandomPage, but bounded by
// the pages TMDB actually has: page 1 is fetched first for its total_pages, and
// a second call fetches the random page when that is not page 1. A pick past
// the last page would come back empty and read as "nothing matches".
//
// genre narrows the recipe the same way a client's genre pick does on the
// addon path — a collection folder's per-reference genre is previewed through
// it.
func (c *TMDBClient) PreviewCatalog(ctx context.Context, catalogType, paramsJSON, genre string) (items []PreviewItem, totalResults int, randomized bool, err error) {
	endpoint, err := catalogEndpoint(catalogType)
	if err != nil {
		return nil, 0, false, err
	}

	query, randomized, err := buildDiscoverQuery(catalogType, paramsJSON)
	if err != nil {
		return nil, 0, false, err
	}
	if err := c.applyGenrePick(ctx, query, catalogType, paramsJSON, genre); err != nil {
		return nil, 0, randomized, err
	}
	query.Set("page", "1")

	resp, err := c.discover(ctx, endpoint, query)
	if err != nil {
		return nil, 0, randomized, err
	}

	if randomized && resp.TotalPages > 1 {
		// Shuffling a preview is cosmetic, so a non-cryptographic source is
		// what this wants.
		if page := rand.IntN(min(resp.TotalPages, maxRandomPage)) + 1; page > 1 { //nolint:gosec // G404
			query.Set("page", strconv.Itoa(page))
			if resp, err = c.discover(ctx, endpoint, query); err != nil {
				return nil, 0, randomized, err
			}
		}
	}

	items = make([]PreviewItem, 0, len(resp.Results))
	for _, item := range resp.Results {
		items = append(items, tmdbItemToPreview(catalogType, item))
	}
	return items, resp.TotalResults, randomized, nil
}

func tmdbItemToPreview(catalogType string, item tmdbDiscoverItem) PreviewItem {
	// TMDB names the same field differently per media type: movies have
	// title/release_date, series have name/first_air_date. Same split
	// tmdbItemToMeta makes.
	name, date := item.Title, item.ReleaseDate
	if catalogType == "series" {
		name, date = item.Name, item.FirstAirDate
	}

	var poster string
	if item.PosterPath != "" {
		poster = tmdbImageBaseURL + item.PosterPath
	}

	return PreviewItem{
		TMDBID: item.ID,
		Title:  name,
		Year:   releaseYear(date),
		Poster: poster,
	}
}
