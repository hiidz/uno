package provider

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// exportPath is the path of the network export published for the day offset
// days from today (UTC).
func exportPath(offset int) string {
	return "/exports/tv_network_ids_" + time.Now().UTC().AddDate(0, 0, offset).Format("01_02_2006") + ".json.gz"
}

// fakeNetworkTMDB serves a network export plus the TMDB routes a network
// search reaches. Today's export answers 403 (not yet published) and
// yesterday's lists entries, or answers 403 too when entries is nil.
// /network/{id} knows the ids in counts (id 1 with origin country "US") and
// answers 404 for the rest; /discover/tv answers with_networks=<id> with
// counts[id] as total_results, or 500 for failID. hits counts requests per
// path, and per "/discover/tv?<id>" for discover.
func fakeNetworkTMDB(t *testing.T, entries []networkEntry, counts map[int]int, failID int) (*TMDBClient, func(key string) int) {
	t.Helper()
	names := map[int]string{}
	for _, e := range entries {
		names[e.ID] = e.Name
	}
	var mu sync.Mutex
	seen := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path
		if key == "/discover/tv" {
			key += "?" + r.URL.Query().Get("with_networks")
		}
		mu.Lock()
		seen[key]++
		mu.Unlock()

		switch {
		case r.URL.Path == exportPath(-1) && entries != nil:
			zw := gzip.NewWriter(w)
			for _, e := range entries {
				line, _ := json.Marshal(e)
				fmt.Fprintf(zw, "%s\n", line)
			}
			_ = zw.Close()
		case strings.HasPrefix(r.URL.Path, "/exports/"):
			w.WriteHeader(http.StatusForbidden)
		case strings.HasPrefix(r.URL.Path, "/network/"):
			id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/network/"))
			if _, ok := counts[id]; !ok {
				http.NotFound(w, r)
				return
			}
			country := ""
			if id == 1 {
				country = "US"
			}
			fmt.Fprintf(w, `{"id":%d,"name":%q,"origin_country":%q}`, id, names[id], country)
		case r.URL.Path == "/discover/tv":
			id, _ := strconv.Atoi(r.URL.Query().Get("with_networks"))
			if id == failID {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			fmt.Fprintf(w, `{"page":1,"results":[],"total_pages":1,"total_results":%d}`, counts[id])
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewTMDBClient("key")
	c.baseURL = srv.URL
	c.exportsURL = srv.URL + "/exports"
	return c, func(key string) int {
		mu.Lock()
		defer mu.Unlock()
		return seen[key]
	}
}

// TestSearchNetworksRanksAndFilters covers the whole search: a mid-word
// match is never considered, a name TMDB no longer has a network for and a
// network under minMatchTitles are dropped, and the rest are ordered by
// series count, named and placed from the by-id lookup.
func TestSearchNetworksRanksAndFilters(t *testing.T) {
	entries := []networkEntry{
		{1, "HBO"},
		{2, "Beachbody"},
		{3, "Cinemax HBO"},
		{4, "HBO"},
		{5, "HBO"},
		{6, "HBO Max"},
		{7, "HBO Go"},
	}
	counts := map[int]int{1: 376, 2: 999, 3: 50, 5: 1, 6: 120, 7: 5}
	c, hits := fakeNetworkTMDB(t, entries, counts, 0)

	got, err := c.SearchNetworks(t.Context(), "  hbo ")
	if err != nil {
		t.Fatalf("SearchNetworks: %v", err)
	}
	want := []NetworkMatch{
		{ID: 1, Name: "HBO", OriginCountry: "US", TitleCount: 376},
		{ID: 6, Name: "HBO Max", TitleCount: 120},
		{ID: 3, Name: "Cinemax HBO", TitleCount: 50},
		{ID: 7, Name: "HBO Go", TitleCount: 5},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SearchNetworks = %+v, want %+v", got, want)
	}
	if n := hits("/network/2") + hits("/discover/tv?2"); n != 0 {
		t.Fatalf("mid-word match looked up %d times, want 0", n)
	}
	if n := hits("/discover/tv?4"); n != 0 {
		t.Fatalf("network TMDB no longer has counted %d times, want 0", n)
	}
}

// TestSearchNetworksCountsOnlyTheBestMatches covers the candidate cap: exact
// names outrank prefixes, the lower id wins a tie, and nothing past the first
// maxCountedMatches is looked up or counted.
func TestSearchNetworksCountsOnlyTheBestMatches(t *testing.T) {
	var entries []networkEntry
	counts := map[int]int{}
	for id := 1; id <= maxCountedMatches+2; id++ {
		name := "HBO Extra"
		if id > maxCountedMatches {
			name = "HBO" // exact, so these two are counted ahead of the rest
		}
		entries = append(entries, networkEntry{id, name})
		counts[id] = 10
	}
	c, hits := fakeNetworkTMDB(t, entries, counts, 0)

	if _, err := c.SearchNetworks(t.Context(), "HBO"); err != nil {
		t.Fatalf("SearchNetworks: %v", err)
	}
	for id := 1; id <= maxCountedMatches+2; id++ {
		want := 1
		if id == maxCountedMatches-1 || id == maxCountedMatches {
			want = 0
		}
		if n := hits("/network/" + strconv.Itoa(id)); n != want {
			t.Fatalf("network %d looked up %d times, want %d", id, n, want)
		}
	}
}

// TestSearchNetworksExportFallbackAndMemo covers the export: today's
// unpublished file falls back to yesterday's, and the list is memoized so a
// second search fetches neither.
func TestSearchNetworksExportFallbackAndMemo(t *testing.T) {
	c, hits := fakeNetworkTMDB(t, []networkEntry{{1, "Netflix"}}, map[int]int{1: 2886}, 0)

	for range 2 {
		got, err := c.SearchNetworks(t.Context(), "netflix")
		if err != nil {
			t.Fatalf("SearchNetworks: %v", err)
		}
		if len(got) != 1 || got[0].ID != 1 {
			t.Fatalf("SearchNetworks = %+v, want Netflix", got)
		}
	}
	for _, path := range []string{exportPath(0), exportPath(-1)} {
		if n := hits(path); n != 1 {
			t.Fatalf("%s requested %d times, want 1", path, n)
		}
	}
}

// TestSearchNetworksExportMissing covers neither day's export being there:
// the search fails as upstream's fault, and the failure is not cached.
func TestSearchNetworksExportMissing(t *testing.T) {
	c, hits := fakeNetworkTMDB(t, nil, nil, 0)

	for range 2 {
		_, err := c.SearchNetworks(t.Context(), "netflix")
		if err == nil || errors.Is(err, ErrInvalidParams) || errors.Is(err, ErrNotFound) {
			t.Fatalf("SearchNetworks error = %v, want an upstream error", err)
		}
	}
	if n := hits(exportPath(-1)); n != 2 {
		t.Fatalf("yesterday's export requested %d times, want 2 (a failure must not be cached)", n)
	}
}

// TestSearchNetworksFailsOnCountFailure covers a count that fails: the
// search fails rather than dropping that result.
func TestSearchNetworksFailsOnCountFailure(t *testing.T) {
	c, _ := fakeNetworkTMDB(t, []networkEntry{{1, "HBO"}, {2, "HBO Max"}}, map[int]int{1: 376, 2: 120}, 2)

	got, err := c.SearchNetworks(t.Context(), "hbo")
	if err == nil {
		t.Fatalf("SearchNetworks = %+v, want an error", got)
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidParams) {
		t.Fatalf("SearchNetworks error = %v, want neither ErrNotFound nor ErrInvalidParams", err)
	}
}

// TestSearchNetworksRejectsBlankQuery covers a blank query: rejected before
// anything is fetched.
func TestSearchNetworksRejectsBlankQuery(t *testing.T) {
	c, hits := fakeNetworkTMDB(t, []networkEntry{{1, "HBO"}}, map[int]int{1: 376}, 0)

	if _, err := c.SearchNetworks(t.Context(), "   "); !errors.Is(err, ErrInvalidParams) {
		t.Fatalf("SearchNetworks error = %v, want ErrInvalidParams", err)
	}
	if n := hits(exportPath(0)) + hits(exportPath(-1)); n != 0 {
		t.Fatalf("export requested %d times, want 0", n)
	}
}

func TestNameMatchRank(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		wantRank    int
		wantOK      bool
	}{
		{"HBO", "hbo", 0, true},
		{"HBO Max", "hbo", 1, true},
		{"Cinemax HBO", "hbo", 2, true},
		{"ABC.com", "com", 2, true},
		{"HBO España", "españa", 2, true},
		{"Beachbody", "hbo", 0, false},
		{"Netflix", "hbo", 0, false},
	} {
		rank, ok := nameMatchRank(tc.name, tc.query)
		if rank != tc.wantRank || ok != tc.wantOK {
			t.Fatalf("nameMatchRank(%q, %q) = %d, %v; want %d, %v", tc.name, tc.query, rank, ok, tc.wantRank, tc.wantOK)
		}
	}
}
