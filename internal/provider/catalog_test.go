package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// fakeCatalogTMDB serves one discover page with a single item, its
// external_ids, and the genre list. genreListStatus lets a test fail the
// genre-list fetch.
func fakeCatalogTMDB(t *testing.T, item string, genreListStatus int) *TMDBClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/discover/"):
			fmt.Fprintf(w, `{"results":[%s],"total_results":1,"total_pages":1}`, item)
		case strings.HasSuffix(r.URL.Path, "/external_ids"):
			fmt.Fprint(w, `{"imdb_id":"tt0468569"}`)
		case strings.HasPrefix(r.URL.Path, "/genre/"):
			if genreListStatus != http.StatusOK {
				w.WriteHeader(genreListStatus)
				return
			}
			fmt.Fprint(w, `{"genres":[{"id":28,"name":"Action"},{"id":80,"name":"Crime"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewTMDBClient("key")
	c.baseURL = srv.URL
	return c
}

func TestFetchCatalogPageMapsDiscoverFields(t *testing.T) {
	c := fakeCatalogTMDB(t, `{"id":155,"title":"The Dark Knight","overview":"o",
		"poster_path":"/p.jpg","backdrop_path":"/b.jpg","genre_ids":[80,28,9999],
		"release_date":"2008-07-16"}`, http.StatusOK)

	metas, err := c.FetchCatalogPage(context.Background(), "movie", `{}`, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []Meta{{
		ID:          "tt0468569",
		Type:        "movie",
		Name:        "The Dark Knight",
		Poster:      "https://image.tmdb.org/t/p/w500/p.jpg",
		Background:  "https://image.tmdb.org/t/p/w1280/b.jpg",
		Description: "o",
		ReleaseInfo: "2008",
		Released:    "2008-07-16T00:00:00.000Z",
		Genres:      []string{"Crime", "Action"},
	}}
	if !reflect.DeepEqual(metas, want) {
		t.Fatalf("got %+v\nwant %+v", metas, want)
	}
}

func TestFetchCatalogPageServesWithoutGenresWhenGenreListFails(t *testing.T) {
	c := fakeCatalogTMDB(t, `{"id":155,"title":"t","genre_ids":[28],"release_date":""}`,
		http.StatusInternalServerError)

	metas, err := c.FetchCatalogPage(context.Background(), "movie", `{}`, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].Genres != nil || metas[0].Background != "" || metas[0].Released != "" {
		t.Fatalf("got %+v, want one meta with no genres, background, or released", metas)
	}
}
