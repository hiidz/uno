package provider

import (
	"context"
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

// PreviewCatalog runs a recipe against TMDB and returns one page of tiles,
// saving nothing. It shares catalogEndpoint, buildDiscoverQuery and discover
// with [TMDBClient.FetchCatalogPage] — which is the point, since the
// underscore-to-dot param translation (vote_average_gte -> vote_average.gte)
// and the type->path mapping each have to live in exactly one place — but it
// deliberately skips resolveMetas.
//
// Always page 1, even for a randomized recipe. Honouring the random page would
// make the preview show different titles on every remount, which reads as a bug
// rather than as shuffling. The randomized flag comes back instead so the
// caller can say plainly that this catalog will differ on the TV.
func (c *TMDBClient) PreviewCatalog(ctx context.Context, catalogType, paramsJSON string) (items []PreviewItem, randomized bool, err error) {
	endpoint, err := catalogEndpoint(catalogType)
	if err != nil {
		return nil, false, err
	}

	query, randomized, err := buildDiscoverQuery(catalogType, paramsJSON)
	if err != nil {
		return nil, false, err
	}
	query.Set("page", "1")

	results, err := c.discover(ctx, endpoint, query)
	if err != nil {
		return nil, randomized, err
	}

	items = make([]PreviewItem, 0, len(results))
	for _, item := range results {
		items = append(items, tmdbItemToPreview(catalogType, item))
	}
	return items, randomized, nil
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
