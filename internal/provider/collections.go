package provider

import (
	"cmp"
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Collection is one TMDB collection (a film series such as Star Wars) —
// with_collection takes ID, and the picker shows Name.
type Collection struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// collectionPartsTTL bounds how long a collection's film list is served: a
// collection gains films as they are announced.
const collectionPartsTTL = 24 * time.Hour

// Collection returns the TMDB collection with id, memoized. It serves both recipe validation's existence check and the
// picker's id-to-name resolution. An id TMDB doesn't have wraps ErrNotFound.
func (c *TMDBClient) Collection(ctx context.Context, id int) (Collection, error) {
	return entityByID(ctx, c, c.collections, "/collection/%d", id)
}

// SearchCollections returns the first page of TMDB's collection search for
// query. Not cached: results vary with every keystroke the picker sends.
func (c *TMDBClient) SearchCollections(ctx context.Context, query string) ([]Collection, error) {
	return searchEntities[Collection](ctx, c, "/search/collection", query)
}

// collectionParts returns the films of the TMDB collection with id, from
// /collection/{id}'s parts, cached for collectionPartsTTL.
func (c *TMDBClient) collectionParts(ctx context.Context, id int) ([]tmdbDiscoverItem, error) {
	return c.collectionFilms.load(strconv.Itoa(id), func() ([]tmdbDiscoverItem, error) {
		var out struct {
			Parts []tmdbDiscoverItem `json:"parts"`
		}
		err := c.get(ctx, fmt.Sprintf("/collection/%d", id), nil, &out)
		return out.Parts, err
	})
}

// collectionItems serves a collection recipe: a movie recipe with
// with_collection set, reported by ok. /discover/movie ignores
// with_collection, so the titles are the collection's own parts, in release
// order (undated last, ties by id), narrowed to genre when it names one of
// GenreExtraOptions, and shuffled instead when the recipe is randomized. The
// whole collection is one page.
func (c *TMDBClient) collectionItems(ctx context.Context, p CatalogParams, catalogType, paramsJSON, genre string) (items []tmdbDiscoverItem, ok bool, err error) {
	movie, isMovie := p.(*TMDBMovieParams)
	if !isMovie || movie.WithCollection == "" {
		return nil, false, nil
	}
	id, err := strconv.Atoi(movie.WithCollection)
	if err != nil {
		return nil, true, fmt.Errorf("%w: with_collection %q is not a numeric id", ErrInvalidParams, movie.WithCollection)
	}
	parts, err := c.collectionParts(ctx, id)
	if err != nil {
		return nil, true, err
	}

	genreID, picked, err := c.pickedGenreID(ctx, catalogType, paramsJSON, genre)
	if err != nil {
		return nil, true, err
	}
	if picked {
		parts = slices.DeleteFunc(parts, func(it tmdbDiscoverItem) bool {
			return !slices.Contains(it.GenreIDs, genreID)
		})
	}

	if movie.Randomized {
		// Shuffling a catalog row is cosmetic, so a non-cryptographic
		// source is what this wants.
		rand.Shuffle(len(parts), func(i, j int) { parts[i], parts[j] = parts[j], parts[i] }) //nolint:gosec // G404
	} else {
		slices.SortFunc(parts, byReleaseDate)
	}
	return parts, true, nil
}

// byReleaseDate orders movies by release_date ascending, undated ones last,
// and ties by TMDB id.
func byReleaseDate(a, b tmdbDiscoverItem) int {
	if (a.ReleaseDate == "") != (b.ReleaseDate == "") {
		if a.ReleaseDate == "" {
			return 1
		}
		return -1
	}
	return cmp.Or(strings.Compare(a.ReleaseDate, b.ReleaseDate), cmp.Compare(a.ID, b.ID))
}
