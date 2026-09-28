package provider

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
)

// CanonicalParams returns params in the canonical form a stored recipe of
// catalogProvider and catalogType takes: decoded as the recipe's params
// struct, so only its known keys survive and zero values, which every field
// reads as unset, are dropped, then encoded compact with its keys sorted. Two
// encodings of one recipe give the same bytes, which is what lets catalogs
// with the same recipe share one recipes row. The form is frozen in the
// vault's migrations: changing it needs a migration that rewrites every
// stored recipe.
func CanonicalParams(catalogType, catalogProvider, params string) (string, error) {
	p, err := decodeRecipe(catalogType, catalogProvider, params)
	if err != nil {
		return "", err
	}
	return sortedJSON(p)
}

// decodeRecipe decodes params as the recipe of catalogProvider and
// catalogType. TMDB is the one provider this package has recipes for, so any
// other is an error.
func decodeRecipe(catalogType, catalogProvider, params string) (CatalogParams, error) {
	if catalogProvider != "tmdb" {
		return nil, fmt.Errorf("provider: no recipes for provider %q", catalogProvider)
	}
	return DecodeParams(catalogType, params)
}

// sortedJSON encodes v, then re-encodes the result with its keys sorted.
func sortedJSON(v any) (string, error) {
	encoded, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("provider: encode params: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return "", fmt.Errorf("provider: encode params: %w", err)
	}
	sorted, err := json.Marshal(fields)
	if err != nil {
		return "", fmt.Errorf("provider: encode params: %w", err)
	}
	return string(sorted), nil
}

// SameRecipe reports, as an error naming the first difference, whether
// before and after would fetch different titles as the recipe of
// catalogProvider and catalogType: the discover query, the shuffle, the TMDB
// collection, or the genres the manifest offers to narrow it by. It makes no
// TMDB call. The migration dry run runs it over every catalog's params before
// and after they were made canonical.
func SameRecipe(catalogType, catalogProvider, before, after string) error {
	a, err := readRecipeFacts(catalogType, catalogProvider, before)
	if err != nil {
		return fmt.Errorf("before: %w", err)
	}
	b, err := readRecipeFacts(catalogType, catalogProvider, after)
	if err != nil {
		return fmt.Errorf("after: %w", err)
	}
	for _, fact := range []struct{ name, a, b string }{
		{"discover query", a.query, b.query},
		{"shuffle", a.randomized, b.randomized},
		{"collection", a.collection, b.collection},
		{"genre options", genreOptionIDs(a, b), genreOptionIDs(b, a)},
	} {
		if fact.a != fact.b {
			return fmt.Errorf("%s differs: %q before, %q after", fact.name, fact.a, fact.b)
		}
	}
	return nil
}

// recipeFacts is what SameRecipe compares of one recipe.
type recipeFacts struct {
	query, randomized, collection string
	withGenres, withoutGenres     string
}

// readRecipeFacts decodes params as the recipe of catalogProvider and
// catalogType and reads off its recipeFacts. The genre filters are decoded
// the way GenreExtraOptions decodes them.
func readRecipeFacts(catalogType, catalogProvider, params string) (recipeFacts, error) {
	p, err := decodeRecipe(catalogType, catalogProvider, params)
	if err != nil {
		return recipeFacts{}, err
	}
	with, without, err := genreFilters(params)
	if err != nil {
		return recipeFacts{}, err
	}
	facts := recipeFacts{
		query:         p.DiscoverQuery().Encode(),
		randomized:    strconv.FormatBool(p.IsRandomized()),
		withGenres:    with,
		withoutGenres: without,
	}
	if movie, ok := p.(*TMDBMovieParams); ok {
		facts.collection = movie.WithCollection
	}
	return facts, nil
}

// genreOptionIDs is the ids genreChoices keeps of f's genre filters, over
// every genre id f or other names plus one neither names: the genres the two
// could disagree about. Two recipes offer the same genre options from any
// genre list exactly when this is the same for both.
func genreOptionIDs(f, other recipeFacts) string {
	ids := map[int]bool{-1: true}
	for _, list := range []string{f.withGenres, f.withoutGenres, other.withGenres, other.withoutGenres} {
		maps.Copy(ids, genreIDSet(list))
	}
	genres := make([]Genre, 0, len(ids))
	for _, id := range slices.Sorted(maps.Keys(ids)) {
		genres = append(genres, Genre{ID: id})
	}
	kept := genreChoices(genres, f.withGenres, f.withoutGenres)
	out := make([]int, len(kept))
	for i, g := range kept {
		out[i] = g.ID
	}
	return fmt.Sprint(out)
}
