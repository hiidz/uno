package provider

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
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
// Every field is filled from the discover item (or collection part) alone, with no per-item
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

	// externalIDsConcurrency bounds how many external_ids lookups are in
	// flight at once per catalog page. resolveMetas starts one goroutine
	// per item either way; this is the ceiling on the TMDB requests they
	// make, which is what the rate that matters is measured in.
	externalIDsConcurrency = 8

	// maxRandomPage bounds "randomized" catalogs to TMDB's first 20 pages.
	// Discover result relevance falls off fast past that; picking from the
	// full total_pages range (which can run into the hundreds) would mostly
	// surface obscure, poorly-populated entries rather than a good surprise.
	maxRandomPage = 20
)

// tmdbMediaType maps Uno/Stremio's catalog vocabulary ("movie", "series") to
// TMDB's own path segment ("movie", "tv"), used wherever this package calls
// a TMDB sub-resource directly — external_ids, Genres, Certifications,
// WatchProviders, and resolveMetas. The discover call itself doesn't need
// this since catalogs.endpoint already carries TMDB's literal word for
// itself.
var tmdbMediaType = map[string]string{
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

// CatalogPage is one page of a catalog's Stremio-shaped metas, and whether
// TMDB has pages past it. A page can hold no metas and still have more: every
// title on it may have lacked an IMDB id (resolveMetas).
type CatalogPage struct {
	Metas []Meta
	More  bool
}

// FetchCatalogPage runs a stored catalog's recipe against TMDB and returns
// one page of Stremio-shaped metas. catalogType ("movie"/"series") selects
// both the TMDB discover path (via catalogEndpoint) and which params shape
// to decode. genre is the client's pick from the catalog's genre extra, by
// name; "", GenreExtraAll, or a name not in GenreExtraOptions leaves the
// recipe unfiltered.
//
// page is ignored in favor of a random pick within the first maxRandomPage
// when the recipe's Randomized flag is set. A narrow recipe can have fewer
// pages than that, and a pick past its last one comes back empty, so an
// empty randomized page falls back to page 1 — a second discover call only
// when the pick overshoots, unlike [TMDBClient.PreviewCatalog], which pays
// for TMDB's total_pages up front.
//
// A collection recipe skips discover (see collectionItems): page 1 is the
// whole collection and every later page is empty, so no page has more.
//
// A discover page is served from the client's page cache (pageCache), keyed
// by its discover request once the genre pick and the page, random or not,
// are in it, so profiles showing the same recipe share one fetch.
func (c *TMDBClient) FetchCatalogPage(ctx context.Context, catalogType, paramsJSON, genre string, page int) (CatalogPage, error) {
	endpoint, err := catalogEndpoint(catalogType)
	if err != nil {
		return CatalogPage{}, err
	}

	p, err := DecodeParams(catalogType, paramsJSON)
	if err != nil {
		return CatalogPage{}, err
	}
	if metas, ok, err := c.collectionPage(ctx, p, catalogType, paramsJSON, genre, page); ok {
		return CatalogPage{Metas: metas}, err
	}

	query := p.DiscoverQuery()
	if err := c.applyGenrePick(ctx, query, catalogType, paramsJSON, genre); err != nil {
		return CatalogPage{}, err
	}
	d := discoverPage{client: c, catalogType: catalogType, endpoint: endpoint, query: query}
	d.pick(page, p.IsRandomized())
	return c.pages.load(ctx, d.key(), d.fetch)
}

// collectionPage is FetchCatalogPage for a collection recipe, whose page 1 is
// the whole collection (see collectionItems). ok is false for any other
// recipe.
func (c *TMDBClient) collectionPage(ctx context.Context, p CatalogParams, catalogType, paramsJSON, genre string, page int) ([]Meta, bool, error) {
	items, ok, err := c.collectionItems(ctx, p, catalogType, paramsJSON, genre)
	if !ok || err != nil || page > 1 {
		return nil, ok, err
	}
	metas, err := c.resolveMetas(ctx, catalogType, items, c.genreNames(ctx, catalogType))
	return metas, true, err
}

// discoverPage is one catalog page's discover request.
type discoverPage struct {
	client      *TMDBClient
	catalogType string
	endpoint    string
	query       url.Values
	// fallback marks a random pick past page 1, which falls back to page 1
	// when it comes back empty.
	fallback bool
}

// pick sets the page the request asks for: page, or a random one within
// maxRandomPage for a randomized recipe. A narrow recipe can have fewer
// pages than that, so a random pick past page 1 is marked to fall back.
func (d *discoverPage) pick(page int, randomized bool) {
	if randomized {
		// Shuffling a catalog row is cosmetic, so a non-cryptographic
		// source is what this wants.
		page = rand.IntN(maxRandomPage) + 1 //nolint:gosec // G404
		d.fallback = page != 1
	}
	d.query.Set("page", strconv.Itoa(page))
}

// key is the page cache's key for the request: its path and sorted query,
// taken before get adds the API key.
func (d discoverPage) key() string {
	return d.endpoint + "?" + d.query.Encode()
}

// fetch runs the request and resolves its items to metas, the page the
// cache holds under key. A page served without genres, because the genre
// list failed to load, is not kept: the cache would share it with every
// profile showing the recipe.
func (d discoverPage) fetch(ctx context.Context) (CatalogPage, bool, error) {
	resp, err := d.results(ctx)
	if err != nil {
		return CatalogPage{}, false, err
	}
	names := d.client.genreNames(ctx, d.catalogType)
	metas, err := d.client.resolveMetas(ctx, d.catalogType, resp.Results, names)
	page, _ := strconv.Atoi(d.query.Get("page"))
	return CatalogPage{Metas: metas, More: page < resp.TotalPages}, names != nil, err
}

// results is the request's discover answer, from page 1 when a random pick
// that falls back comes back empty. TMDB's total_pages in it tells the addon
// whether the catalog runs past this page.
func (d discoverPage) results(ctx context.Context) (tmdbDiscoverResponse, error) {
	resp, err := d.client.discover(ctx, d.endpoint, d.query)
	if err != nil || !d.fallback || len(resp.Results) > 0 {
		return resp, err
	}
	d.query.Set("page", "1")
	return d.client.discover(ctx, d.endpoint, d.query)
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
//
// An item TMDB has no IMDB id for is dropped, and so is one whose
// external_ids TMDB answers 404 for: this addon's meta ids are IMDB ids, so
// an item without one can't be represented, and neither fact changes
// between requests. Any other failed lookup fails the whole page, because
// the caller serves a successful page with a three-hour cache header — a
// short page from a TMDB blip would stick on the client for that long. The
// first such error wins and cancels the lookups still in flight.
func (c *TMDBClient) resolveMetas(ctx context.Context, catalogType string, items []tmdbDiscoverItem, genreNames map[int]string) ([]Meta, error) {
	mediaType, ok := tmdbMediaType[catalogType]
	if !ok {
		return nil, fmt.Errorf("%w: got %q", ErrInvalidCatalogType, catalogType)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	metas := make([]Meta, len(items))
	sem := make(chan struct{}, externalIDsConcurrency)
	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
	)
	for i, item := range items {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			imdbID, err := c.imdbID(ctx, mediaType, item.ID)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					return // as permanent as an empty imdb_id; drop the item
				}
				once.Do(func() {
					firstErr = err
					cancel()
				})
				return
			}
			if imdbID == "" {
				return // leaves metas[i] zero-valued; filtered out below
			}
			metas[i] = tmdbItemToMeta(catalogType, imdbID, item, genreNames)
		})
	}
	wg.Wait()

	if firstErr != nil {
		return nil, fmt.Errorf("provider: resolve IMDB ids for %s page: %w", catalogType, firstErr)
	}

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
