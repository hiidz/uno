package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

// fakeCollectionTMDB serves collection 10 with five parts out of release
// order — two sharing a date and one undated — plus the movie genre list and
// each part's external_ids. hits reports how many requests a path prefix got.
func fakeCollectionTMDB(t *testing.T) (c *TMDBClient, hits func(prefix string) int) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch {
		case r.URL.Path == "/collection/10":
			fmt.Fprint(w, `{"id":10,"name":"Saga","parts":[
				{"id":3,"title":"C","release_date":"2005-05-19","genre_ids":[28,12]},
				{"id":1,"title":"A","release_date":"1977-05-25","genre_ids":[28]},
				{"id":5,"title":"E","release_date":"","genre_ids":[12]},
				{"id":4,"title":"D","release_date":"1980-05-21","genre_ids":[12]},
				{"id":2,"title":"B","release_date":"1980-05-21","genre_ids":[28]}]}`)
		case r.URL.Path == "/genre/movie/list":
			fmt.Fprint(w, `{"genres":[{"id":28,"name":"Action"},{"id":12,"name":"Adventure"}]}`)
		case strings.HasSuffix(r.URL.Path, "/external_ids"):
			var id int
			_, _ = fmt.Sscanf(r.URL.Path, "/movie/%d/external_ids", &id)
			fmt.Fprintf(w, `{"imdb_id":"tt%07d"}`, id)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c = NewTMDBClient("key")
	c.baseURL = srv.URL
	return c, func(prefix string) int {
		mu.Lock()
		defer mu.Unlock()
		n := 0
		for _, p := range paths {
			if strings.HasPrefix(p, prefix) {
				n++
			}
		}
		return n
	}
}

func metaNames(metas []Meta) []string {
	names := make([]string, 0, len(metas))
	for _, m := range metas {
		names = append(names, m.Name)
	}
	return names
}

func previewTitles(items []PreviewItem) []string {
	titles := make([]string, 0, len(items))
	for _, it := range items {
		titles = append(titles, it.Title)
	}
	return titles
}

// TestFetchCatalogPageCollection covers the addon path of a collection recipe:
// release order with undated last and ties by id, a genre pick filtering the
// parts locally, and everything on page 1.
func TestFetchCatalogPageCollection(t *testing.T) {
	const params = `{"with_collection":"10"}`
	for _, tc := range []struct {
		name  string
		genre string
		page  int
		want  []string
	}{
		{"release order", "", 1, []string{"A", "B", "D", "C", "E"}},
		{"all genres", GenreExtraAll, 1, []string{"A", "B", "D", "C", "E"}},
		{"genre pick", "Adventure", 1, []string{"D", "C", "E"}},
		{"unknown genre", "Western", 1, []string{"A", "B", "D", "C", "E"}},
		{"page 2 empty", "", 2, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, hits := fakeCollectionTMDB(t)
			metas, err := c.FetchCatalogPage(t.Context(), "movie", params, tc.genre, tc.page)
			if err != nil {
				t.Fatal(err)
			}
			if got := metaNames(metas); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if n := hits("/discover/"); n != 0 {
				t.Fatalf("discover requested %d times, want never", n)
			}
		})
	}
}

// TestFetchCatalogPageCollectionRandomized covers a shuffled collection: the
// same films, in whatever order.
func TestFetchCatalogPageCollectionRandomized(t *testing.T) {
	c, hits := fakeCollectionTMDB(t)
	metas, err := c.FetchCatalogPage(t.Context(), "movie", `{"with_collection":"10","randomized":true}`, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	got := metaNames(metas)
	slices.Sort(got)
	if want := []string{"A", "B", "C", "D", "E"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v in any order", got, want)
	}
	if n := hits("/discover/"); n != 0 {
		t.Fatalf("discover requested %d times, want never", n)
	}
}

// TestPreviewCatalogCollection covers the preview path: the tiles are the
// whole (genre-filtered) collection and the total is their count.
func TestPreviewCatalogCollection(t *testing.T) {
	for _, tc := range []struct {
		genre     string
		want      []string
		wantTotal int
	}{
		{"", []string{"A", "B", "D", "C", "E"}, 5},
		{"Action", []string{"A", "B", "C"}, 3},
	} {
		t.Run("genre="+tc.genre, func(t *testing.T) {
			c, hits := fakeCollectionTMDB(t)
			items, total, randomized, err := c.PreviewCatalog(t.Context(), "movie", `{"with_collection":"10"}`, tc.genre)
			if err != nil {
				t.Fatal(err)
			}
			if got := previewTitles(items); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if total != tc.wantTotal || randomized {
				t.Fatalf("total %d, randomized %v; want %d, false", total, randomized, tc.wantTotal)
			}
			if n := hits("/discover/"); n != 0 {
				t.Fatalf("discover requested %d times, want never", n)
			}
		})
	}
}

// TestCollectionPartsAreMemoized covers the collection-parts cache: repeat pages and
// previews of the same collection fetch it once.
func TestCollectionPartsAreMemoized(t *testing.T) {
	c, hits := fakeCollectionTMDB(t)
	for range 2 {
		if _, err := c.FetchCatalogPage(t.Context(), "movie", `{"with_collection":"10"}`, "", 1); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := c.PreviewCatalog(t.Context(), "movie", `{"with_collection":"10","randomized":true}`, "Action"); err != nil {
			t.Fatal(err)
		}
	}
	if n := hits("/collection/"); n != 1 {
		t.Fatalf("/collection/10 requested %d times, want 1", n)
	}
}

// TestMovieValidateCollectionExcludesOtherFields sets each TMDBMovieParams
// field in turn beside with_collection, found by reflection so a field added
// later is covered too: only randomized may accompany it, and a rejection
// names the offending field.
func TestMovieValidateCollectionExcludesOtherFields(t *testing.T) {
	if err := (TMDBMovieParams{WithCollection: "10", TMDBCommonParams: TMDBCommonParams{BaseParams: BaseParams{Randomized: true}}}).Validate(); err != nil {
		t.Fatalf("with_collection + randomized: %v, want accepted", err)
	}

	var leaves func(v reflect.Value, visit func(field reflect.Value, name string))
	leaves = func(v reflect.Value, visit func(field reflect.Value, name string)) {
		for i := range v.NumField() {
			f := v.Type().Field(i)
			if f.Anonymous {
				leaves(v.Field(i), visit)
				continue
			}
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			visit(v.Field(i), name)
		}
	}

	var probe TMDBMovieParams
	leaves(reflect.ValueOf(&probe).Elem(), func(_ reflect.Value, name string) {
		if name == "with_collection" || name == "randomized" {
			return
		}
		t.Run(name, func(t *testing.T) {
			p := TMDBMovieParams{WithCollection: "10"}
			leaves(reflect.ValueOf(&p).Elem(), func(field reflect.Value, n string) {
				if n != name {
					return
				}
				switch field.Kind() {
				case reflect.String:
					field.SetString("1")
				case reflect.Int:
					field.SetInt(1)
				case reflect.Float64:
					field.SetFloat(1)
				case reflect.Bool:
					field.SetBool(true)
				default:
					t.Fatalf("unhandled kind %s", field.Kind())
				}
			})
			err := p.Validate()
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("Validate() = %v, want an error naming %s", err, name)
			}
		})
	})
}
