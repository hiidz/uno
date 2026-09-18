package provider

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"time"
)

// ErrInvalidCatalogType is returned when a catalog's Type is neither
// "movie" nor "series".
var ErrInvalidCatalogType = errors.New("invalid catalog type: must be movie or series")

// Meta is one Stremio catalog tile. ID must be an IMDB id (tt...) — Stremio
// resolves meta/stream lookups by that id, not TMDB's own numeric id.
//
// Every field is filled from the discover response alone, with no per-item
// call beyond the IMDB id lookup. Background drives Nuvio's hero backdrop
// and landscape tiles; Genres and Released feed its hero metadata line.
type Meta struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Poster      string   `json:"poster,omitempty"`
	Background  string   `json:"background,omitempty"`
	Description string   `json:"description,omitempty"`
	ReleaseInfo string   `json:"releaseInfo,omitempty"`
	Released    string   `json:"released,omitempty"`
	Genres      []string `json:"genres,omitempty"`
}

const (
	tmdbImageBaseURL = "https://image.tmdb.org/t/p/w500"

	// tmdbBackdropBaseURL serves backdrops at a width fit for a full-screen
	// TV hero; w500 is sized for poster tiles.
	tmdbBackdropBaseURL = "https://image.tmdb.org/t/p/w1280"

	// externalIDsConcurrency bounds how many external_ids lookups run at
	// once per catalog page — unbounded would fire one goroutine per item
	// with no ceiling if a future caller ever asks for more than one
	// TMDB page at a time.
	externalIDsConcurrency = 8

	// maxRandomPage bounds "randomized" catalogs to TMDB's first 20 pages.
	// Discover result relevance falls off fast past that; picking from the
	// full total_pages range (which can run into the hundreds) would mostly
	// surface obscure, poorly-populated entries rather than a good surprise.
	maxRandomPage = 20
)

// externalIDsMediaType maps Uno/Stremio's catalog vocabulary ("movie",
// "series") to TMDB's own path segment for the external_ids lookup
// ("movie", "tv"). The discover call itself doesn't need this — catalogs.endpoint
// already carries TMDB's literal word for itself — but external_ids is a
// fixed sub-resource this package calls directly, so it needs its own
// translation.
var externalIDsMediaType = map[string]string{
	"movie":  "movie",
	"series": "tv",
}

type tmdbDiscoverResponse struct {
	Results []tmdbDiscoverItem `json:"results"`
	// TotalResults is TMDB's count of matches across every page, not just this
	// one — the number a preview needs to say "there are N of these" without
	// claiming the page in hand is the whole answer.
	TotalResults int `json:"total_results"`
	TotalPages   int `json:"total_pages"`
}

type tmdbDiscoverItem struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Name         string `json:"name"`
	Overview     string `json:"overview"`
	PosterPath   string `json:"poster_path"`
	BackdropPath string `json:"backdrop_path"`
	GenreIDs     []int  `json:"genre_ids"`
	ReleaseDate  string `json:"release_date"`
	FirstAirDate string `json:"first_air_date"`
}

type tmdbExternalIDs struct {
	IMDBID string `json:"imdb_id"`
}

// catalogEndpoints maps a catalog's type to the TMDB discover path it runs
// against. Derived here from catalogType rather than accepted from a caller
// (e.g. catalogs.endpoint) — [TMDBClient.get] builds its request as
// baseURL+path, so a caller-supplied path would be a request-forgery
// surface: it lets whoever wrote the row point the server's own outbound
// request, api_key included, anywhere they want. FetchCatalogPage and
// PreviewCatalog both go through catalogEndpoint so there is exactly one
// place this mapping lives.
var catalogEndpoints = map[string]string{
	"movie":  "/discover/movie",
	"series": "/discover/tv",
}

func catalogEndpoint(catalogType string) (string, error) {
	endpoint, ok := catalogEndpoints[catalogType]
	if !ok {
		return "", fmt.Errorf("%w: got %q", ErrInvalidCatalogType, catalogType)
	}
	return endpoint, nil
}

// FetchCatalogPage runs a stored catalog's recipe against TMDB and returns
// one page of Stremio-shaped metas. catalogType ("movie"/"series") selects
// both the TMDB discover path (via catalogEndpoint) and which params shape
// to decode. genre is the client's pick from the catalog's genre extra, by
// name; "", GenreExtraAll, or a name not in GenreExtraOptions leaves the
// recipe unfiltered. page is ignored in favor of a random pick when the
// recipe's Randomized flag is set.
func (c *TMDBClient) FetchCatalogPage(ctx context.Context, catalogType, paramsJSON, genre string, page int) ([]Meta, error) {
	endpoint, err := catalogEndpoint(catalogType)
	if err != nil {
		return nil, err
	}

	query, randomized, err := buildDiscoverQuery(catalogType, paramsJSON)
	if err != nil {
		return nil, err
	}
	if err := c.applyGenrePick(ctx, query, catalogType, paramsJSON, genre); err != nil {
		return nil, err
	}
	if randomized {
		page = rand.Intn(maxRandomPage) + 1
	}
	query.Set("page", strconv.Itoa(page))

	// The addon path serves one page as Stremio metas; it has no notion of
	// "how many total" to report, so the count TMDB hands back alongside the
	// page goes unused here.
	resp, err := c.discover(ctx, endpoint, query)
	if err != nil {
		return nil, err
	}

	return c.resolveMetas(ctx, catalogType, resp.Results, c.genreNames(ctx, catalogType))
}

// genreNames maps TMDB genre ids to names for catalogType. Genres are
// decoration on a tile, so a failed genre-list fetch yields an empty map and
// the page is served without them rather than failing.
func (c *TMDBClient) genreNames(ctx context.Context, catalogType string) map[int]string {
	genres, err := c.Genres(ctx, catalogType)
	if err != nil {
		return nil
	}
	names := make(map[int]string, len(genres))
	for _, g := range genres {
		names[g.ID] = g.Name
	}
	return names
}

// resolveMetas resolves each discover item's TMDB id to an IMDB id
// concurrently (bounded by externalIDsConcurrency) — sequential per-item
// calls would turn one page (~20 items) into twenty round trips end to end.
// Items TMDB has no IMDB id for are dropped: this addon's meta ids are IMDB
// ids, so an item without one can't be represented.
func (c *TMDBClient) resolveMetas(ctx context.Context, catalogType string, items []tmdbDiscoverItem, genreNames map[int]string) ([]Meta, error) {
	mediaType, ok := externalIDsMediaType[catalogType]
	if !ok {
		return nil, fmt.Errorf("%w: got %q", ErrInvalidCatalogType, catalogType)
	}

	metas := make([]Meta, len(items))
	sem := make(chan struct{}, externalIDsConcurrency)
	var wg sync.WaitGroup
	for i, item := range items {
		wg.Add(1)
		go func(i int, item tmdbDiscoverItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			imdbID, err := c.imdbID(ctx, mediaType, item.ID)
			if err != nil || imdbID == "" {
				return // leaves metas[i] zero-valued; filtered out below
			}
			metas[i] = tmdbItemToMeta(catalogType, imdbID, item, genreNames)
		}(i, item)
	}
	wg.Wait()

	out := metas[:0]
	for _, m := range metas {
		if m.ID != "" {
			out = append(out, m)
		}
	}
	return out, nil
}

func tmdbItemToMeta(catalogType, imdbID string, item tmdbDiscoverItem, genreNames map[int]string) Meta {
	name := item.Title
	date := item.ReleaseDate
	if catalogType == "series" {
		name = item.Name
		date = item.FirstAirDate
	}

	var poster, background string
	if item.PosterPath != "" {
		poster = tmdbImageBaseURL + item.PosterPath
	}
	if item.BackdropPath != "" {
		background = tmdbBackdropBaseURL + item.BackdropPath
	}

	var genres []string
	for _, id := range item.GenreIDs {
		if n, ok := genreNames[id]; ok {
			genres = append(genres, n)
		}
	}

	return Meta{
		ID:          imdbID,
		Type:        catalogType,
		Name:        name,
		Poster:      poster,
		Background:  background,
		Description: item.Overview,
		ReleaseInfo: releaseYear(date),
		Released:    releasedTimestamp(date),
		Genres:      genres,
	}
}

// releasedTimestamp turns TMDB's "YYYY-MM-DD" into the midnight-UTC ISO 8601
// timestamp Stremio's released field carries ("2008-07-16T00:00:00.000Z");
// Stremio-protocol clients parse it as a full datetime, not a bare date.
// Anything that isn't a valid date yields "".
func releasedTimestamp(date string) string {
	t, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02T15:04:05.000Z")
}

// releaseYear trims TMDB's full "YYYY-MM-DD" date down to just the year —
// Stremio's own convention for releaseInfo (per the real Cinemeta catalog
// response in docs/api/samples/catalog-response.json:
// "releaseInfo":"2008", not the full date).
func releaseYear(date string) string {
	if len(date) < 4 {
		return ""
	}
	return date[:4]
}
