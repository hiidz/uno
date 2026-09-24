// Package provider talks to TMDB on behalf of catalogs and previews:
// building request URLs from a catalog's stored recipe, executing them
// against the TMDB API, and normalizing the results into the shapes the
// Stremio addon protocol expects.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hiidz/uno/internal/jsonwire"
)

const tmdbBaseURL = "https://api.themoviedb.org/3"

// tmdbExportsURL is where TMDB publishes its daily id exports. It needs no
// API key: see networkList.
const tmdbExportsURL = "https://files.tmdb.org/p/exports"

// maxResponseBytes caps how much of a TMDB response this package will read.
// The largest of them is the watch-provider list at a few hundred entries,
// far under this; the cap keeps a malfunctioning upstream from making Uno
// buffer without limit.
const maxResponseBytes = 8 << 20 // 8 MiB

// TMDBClient executes catalog and preview requests against the TMDB API.
type TMDBClient struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
	exportsURL string

	// imdbCache holds tmdbID->IMDB-id lookups, keyed "movie:123"/"tv:456".
	// An id pairing never changes once TMDB has it, so an entry never
	// expires — the cache just saves repeat external_ids round trips across
	// catalog requests. It is filled from the public addon route, so it is
	// capped at maxIMDBCacheEntries and emptied whole once it fills.
	imdbMu    sync.RWMutex
	imdbCache map[string]string

	// The lookup lists TMDB publishes, each cached by its own method: see
	// Genres, Languages, Countries, WatchRegions, WatchProviders and
	// Certifications. Built in NewTMDBClient because a memo needs the clone
	// function for its value type.
	genres         *memo[[]Genre]
	languages      *memo[[]Language]
	countries      *memo[[]Country]
	watchRegions   *memo[[]WatchRegion]
	watchProviders *memo[[]WatchProvider]
	certifications *memo[map[string][]Certification]

	// Single entities looked up by id, keyed by the id: see Company,
	// Keyword, Collection and Network.
	companies   *memo[Company]
	keywords    *memo[Keyword]
	collections *memo[Collection]
	networks    *memo[Network]

	// networkIDs holds the daily network export under one key: see
	// networkList.
	networkIDs *memo[[]networkEntry]

	// collectionFilms holds each collection's films, keyed by collection id:
	// see collectionParts.
	collectionFilms *memo[[]tmdbDiscoverItem]

	// titleCounts holds each company's or network's title count for one
	// catalog type, keyed "with_companies:movie:123"/"with_networks:series:213":
	// see titleCount.
	titleCounts *memo[int]
}

// maxEntityCacheEntries bounds each memo keyed by an id a caller supplies:
// the per-id companies, keywords, collections and networks, and the title
// counts. Each entry is an id and a short name or a count, so the bound
// holds each one to about a megabyte while sitting far above what this
// deployment's users look up between restarts.
const maxEntityCacheEntries = 10_000

// maxCollectionPartsEntries bounds collectionFilms. An entry is a
// collection's whole film list, overviews included — tens of kilobytes for
// a long series — so it is held to fewer entries than the id memos.
const maxCollectionPartsEntries = 1_000

// maxIMDBCacheEntries bounds the tmdbID->IMDB-id cache. Its key space is
// every title TMDB has, reachable one entry per item through the
// unauthenticated addon route, so it needs a ceiling. Reaching it empties
// the map rather than evicting a victim: an entry costs one external_ids
// call to re-derive, which is not worth an eviction policy.
const maxIMDBCacheEntries = 100_000

// watchProviderTTL bounds how long a cached watch-provider list is served.
// Unlike the ISO tables beside it, this one moves: services launch in and
// leave markets, and without an expiry a long-running process would reject
// a newly-added provider id for as long as it stayed up.
const watchProviderTTL = 24 * time.Hour

// NewTMDBClient builds a TMDBClient that authenticates requests with
// apiKey.
func NewTMDBClient(apiKey string) *TMDBClient {
	return &TMDBClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		apiKey:     apiKey,
		baseURL:    tmdbBaseURL,
		exportsURL: tmdbExportsURL,
		imdbCache:  make(map[string]string),

		genres:         newMemo(0, slices.Clone[[]Genre]),
		languages:      newMemo(0, slices.Clone[[]Language]),
		countries:      newMemo(0, slices.Clone[[]Country]),
		watchRegions:   newMemo(0, slices.Clone[[]WatchRegion]),
		watchProviders: newMemo(watchProviderTTL, slices.Clone[[]WatchProvider]),
		certifications: newMemo(0, cloneCertifications),
		companies:      newBoundedMemo(0, maxEntityCacheEntries, func(v Company) Company { return v }),
		keywords:       newBoundedMemo(0, maxEntityCacheEntries, func(v Keyword) Keyword { return v }),
		collections:    newBoundedMemo(0, maxEntityCacheEntries, func(v Collection) Collection { return v }),
		networks:       newBoundedMemo(0, maxEntityCacheEntries, func(v Network) Network { return v }),
		networkIDs:     newMemo(networkExportTTL, slices.Clone[[]networkEntry]),

		collectionFilms: newBoundedMemo(collectionPartsTTL, maxCollectionPartsEntries, slices.Clone[[]tmdbDiscoverItem]),
		titleCounts:     newBoundedMemo(titleCountsTTL, maxEntityCacheEntries, func(v int) int { return v }),
	}
}

// ErrNotFound marks a TMDB 404 — the resource is absent rather than
// momentarily unreachable, so a retry returns the same answer. Separated
// from the other statuses because resolveMetas drops the one item whose
// external_ids TMDB doesn't have instead of failing the page over it, and
// because a lookup of one company, keyword, collection or network by id
// answers a 404 with a 404.
var ErrNotFound = errors.New("provider: TMDB has no such resource")

// get is the shared low-level TMDB call: attach the API key, require a 200,
// decode the body. discover and external_ids both go through this.
func (c *TMDBClient) get(ctx context.Context, path string, query url.Values, out any) error {
	if query == nil {
		query = url.Values{}
	}
	query.Set("api_key", c.apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+query.Encode(), nil)
	if err != nil {
		return fmt.Errorf("provider: build request for %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("provider: fetch %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %s", ErrNotFound, path)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("provider: TMDB returned status %d for %s", resp.StatusCode, path)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); err != nil {
		return fmt.Errorf("provider: decode response for %s: %w", path, err)
	}
	return nil
}

func (c *TMDBClient) discover(ctx context.Context, endpoint string, query url.Values) (tmdbDiscoverResponse, error) {
	var out tmdbDiscoverResponse
	if err := c.get(ctx, endpoint, query, &out); err != nil {
		return tmdbDiscoverResponse{}, err
	}
	return out, nil
}

// entityByID fetches the one TMDB entity at pathFormat's id through m, so
// each id costs one round trip per process. A 404 comes back wrapping
// ErrNotFound and, like every failed load, is not cached. An id below 1 is
// rejected with ErrInvalidParams before TMDB is contacted.
func entityByID[T any](ctx context.Context, c *TMDBClient, m *memo[T], pathFormat string, id int) (T, error) {
	if id < 1 {
		var zero T
		return zero, fmt.Errorf("%w: id %d is not a TMDB id", ErrInvalidParams, id)
	}
	return m.load(strconv.Itoa(id), func() (T, error) {
		var out T
		err := c.get(ctx, fmt.Sprintf(pathFormat, id), url.Values{}, &out)
		return out, err
	})
}

// searchEntities returns the first page of a TMDB /search endpoint's results
// for query. A blank query is rejected with ErrInvalidParams before TMDB is
// contacted.
func searchEntities[T any](ctx context.Context, c *TMDBClient, path, query string) ([]T, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("%w: search query is blank", ErrInvalidParams)
	}
	var out struct {
		Results []T `json:"results"`
	}
	if err := c.get(ctx, path, url.Values{"query": {query}}, &out); err != nil {
		return nil, err
	}
	return jsonwire.OrEmpty(out.Results), nil
}

// imdbID resolves one TMDB id to an IMDB id, consulting the client's cache
// first. A successful lookup is cached even when TMDB has no imdb_id for the
// item (empty string) — that fact doesn't change either, so repeat requests
// for the same title shouldn't keep re-asking. A failed request is not
// cached: a transient error shouldn't be remembered as "no id".
func (c *TMDBClient) imdbID(ctx context.Context, mediaType string, tmdbID int) (string, error) {
	key := mediaType + ":" + strconv.Itoa(tmdbID)

	c.imdbMu.RLock()
	cached, ok := c.imdbCache[key]
	c.imdbMu.RUnlock()
	if ok {
		return cached, nil
	}

	var out tmdbExternalIDs
	if err := c.get(ctx, fmt.Sprintf("/%s/%d/external_ids", mediaType, tmdbID), nil, &out); err != nil {
		return "", err
	}

	c.imdbMu.Lock()
	if len(c.imdbCache) >= maxIMDBCacheEntries {
		c.imdbCache = make(map[string]string)
	}
	c.imdbCache[key] = out.IMDBID
	c.imdbMu.Unlock()
	return out.IMDBID, nil
}
