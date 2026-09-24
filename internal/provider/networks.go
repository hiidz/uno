package provider

import (
	"cmp"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hiidz/uno/internal/jsonwire"
)

// Network is one TMDB TV network — with_networks takes ID, and the picker
// shows Name. OriginCountry may be "".
type Network struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	OriginCountry string `json:"origin_country"`
}

// NetworkMatch is one network search result: the same wire shape as a
// CompanyMatch, so the picker tells same-named networks apart the same way.
// TitleCount is how many series TMDB's discover credits to the network.
type NetworkMatch struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	OriginCountry string `json:"origin_country"`
	TitleCount    int    `json:"title_count"`
}

// networkEntry is one line of TMDB's daily network export.
type networkEntry struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// networkExportTTL bounds how long a fetched network export is served: TMDB
// publishes a new one daily.
const networkExportTTL = 24 * time.Hour

// errExportMissing marks an export date the host answered with anything but
// a 200. TMDB's host answers 403 for a date it hasn't published yet.
var errExportMissing = errors.New("provider: TMDB export not available")

// Network returns the TMDB network with id, memoized. It serves recipe
// validation's existence check, the picker's id-to-name resolution, and each
// search result's origin country. An id TMDB doesn't have wraps ErrNotFound.
func (c *TMDBClient) Network(ctx context.Context, id int) (Network, error) {
	return entityByID(ctx, c, c.networks, "/network/%d", id)
}

// SearchNetworks returns the networks whose name matches query that credit
// at least minMatchTitles series, most series first. TMDB has no network
// search, so names are matched against its daily network export (see
// networkList): an exact name first, then a name starting with query, then a
// word inside the name starting with it, the lower id first within each.
// Only the first maxCountedMatches names are looked up and counted, by
// Network and titleCount; a name TMDB no longer has a network for is
// dropped.
//
// A blank query is rejected before anything is fetched. A failed lookup or
// count fails the whole search rather than dropping the result it was for.
func (c *TMDBClient) SearchNetworks(ctx context.Context, query string) ([]NetworkMatch, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, fmt.Errorf("%w: search query is blank", ErrInvalidParams)
	}
	entries, err := c.networkList(ctx)
	if err != nil {
		return nil, err
	}

	type ranked struct {
		id, rank int
	}
	var hits []ranked
	for _, e := range entries {
		if rank, ok := nameMatchRank(e.Name, query); ok {
			hits = append(hits, ranked{e.ID, rank})
		}
	}
	slices.SortFunc(hits, func(a, b ranked) int {
		return cmp.Or(cmp.Compare(a.rank, b.rank), cmp.Compare(a.id, b.id))
	})
	hits = hits[:min(len(hits), maxCountedMatches)]

	matches := make([]NetworkMatch, len(hits))
	err = forEachMatch(ctx, len(hits), func(ctx context.Context, i int) error {
		n, err := c.Network(ctx, hits[i].id)
		if errors.Is(err, ErrNotFound) {
			return nil // leaves matches[i] with no titles; dropped below
		}
		if err != nil {
			return err
		}
		count, err := c.titleCount(ctx, "with_networks", "series", n.ID)
		matches[i] = NetworkMatch{ID: n.ID, Name: n.Name, OriginCountry: n.OriginCountry, TitleCount: count}
		return err
	})
	if err != nil {
		return nil, err
	}

	matches = slices.DeleteFunc(matches, func(m NetworkMatch) bool { return m.TitleCount < minMatchTitles })
	slices.SortStableFunc(matches, func(a, b NetworkMatch) int { return cmp.Compare(b.TitleCount, a.TitleCount) })
	return jsonwire.OrEmpty(matches), nil
}

// nameMatchRank reports whether name matches the lower-case query and how
// well: 0 for the whole name, 1 for its start, 2 for the start of a later
// word. A query met only mid-word ("hbo" in "Beachbody") is no match.
func nameMatchRank(name, query string) (int, bool) {
	name = strings.ToLower(name)
	if name == query {
		return 0, true
	}
	if strings.HasPrefix(name, query) {
		return 1, true
	}
	for i := 1; i < len(name); i++ {
		if !strings.HasPrefix(name[i:], query) {
			continue
		}
		if r, _ := utf8.DecodeLastRuneInString(name[:i]); !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return 2, true
		}
	}
	return 0, false
}

// networkList returns every network in TMDB's daily export, memoized for
// networkExportTTL. The export is a gzipped file of one {"id","name"} object
// per line on TMDB's public file host, fetched without the API key. Today's
// (UTC) is tried first and yesterday's when today's isn't published yet.
func (c *TMDBClient) networkList(ctx context.Context) ([]networkEntry, error) {
	return c.networkIDs.load("", func() ([]networkEntry, error) {
		today := time.Now().UTC()
		entries, err := c.fetchNetworkExport(ctx, today)
		if errors.Is(err, errExportMissing) {
			entries, err = c.fetchNetworkExport(ctx, today.AddDate(0, 0, -1))
		}
		return entries, err
	})
}

// fetchNetworkExport downloads and decodes the network export published for
// day. An export with no entries is an error, so it is never cached.
func (c *TMDBClient) fetchNetworkExport(ctx context.Context, day time.Time) ([]networkEntry, error) {
	path := "/tv_network_ids_" + day.Format("01_02_2006") + ".json.gz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.exportsURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("provider: build request for %s: %w", path, err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider: fetch %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s answered %d", errExportMissing, path, resp.StatusCode)
	}
	zr, err := gzip.NewReader(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("provider: read %s: %w", path, err)
	}
	defer func() { _ = zr.Close() }()

	var entries []networkEntry
	dec := json.NewDecoder(io.LimitReader(zr, maxResponseBytes))
	for {
		var e networkEntry
		if err := dec.Decode(&e); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("provider: decode %s: %w", path, err)
		}
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("provider: %s lists no networks", path)
	}
	return entries, nil
}
