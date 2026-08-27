// Package provider talks to TMDB on behalf of catalogs and previews:
// building request URLs from a catalog's stored recipe, executing them
// against the TMDB API, and normalizing the results into the shapes the
// Stremio addon protocol expects.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const tmdbBaseURL = "https://api.themoviedb.org/3"

// TMDBClient executes catalog and preview requests against the TMDB API.
type TMDBClient struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string

	// imdbCache holds tmdbID->IMDB-id lookups, keyed "movie:123"/"tv:456".
	// An id pairing never changes once TMDB has it, so this never expires —
	// it just saves repeat external_ids round trips across catalog requests.
	imdbMu    sync.RWMutex
	imdbCache map[string]string
}

// NewTMDBClient builds a TMDBClient that authenticates requests with
// apiKey.
func NewTMDBClient(apiKey string) *TMDBClient {
	return &TMDBClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		apiKey:    apiKey,
		baseURL:   tmdbBaseURL,
		imdbCache: make(map[string]string),
	}
}

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("provider: TMDB returned status %d for %s", resp.StatusCode, path)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("provider: decode response for %s: %w", path, err)
	}
	return nil
}

func (c *TMDBClient) discover(ctx context.Context, endpoint string, query url.Values) ([]tmdbDiscoverItem, error) {
	var out tmdbDiscoverResponse
	if err := c.get(ctx, endpoint, query, &out); err != nil {
		return nil, err
	}
	return out.Results, nil
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
	c.imdbCache[key] = out.IMDBID
	c.imdbMu.Unlock()
	return out.IMDBID, nil
}
