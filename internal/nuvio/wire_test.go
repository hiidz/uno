package nuvio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// keyPaths adds every object key under v to out, as a path from prefix with
// [] for an array element: {"a":[{"b":1}]} is $.a and $.a[].b.
func keyPaths(v any, prefix string, out map[string]bool) {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			out[prefix+"."+k] = true
			keyPaths(x, prefix+"."+k, out)
		}
	case []any:
		for _, x := range v {
			keyPaths(x, prefix+"[]", out)
		}
	}
}

// jsonKeyPaths is keyPaths over raw JSON, added to out.
func jsonKeyPaths(t *testing.T, raw []byte, out map[string]bool) {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decoding %s: %v", raw, err)
	}
	keyPaths(v, "$", out)
}

// TestPushCollectionKeysMatchSamples checks every key a pushed collection
// carries against the collections Nuvio holds in docs/api/samples, which are
// only ever read. The two samples together carry every field Uno writes, so
// there is no exception list: a key missing from both is a renamed field.
// Every optional field is set, so none is skipped as empty.
func TestPushCollectionKeysMatchSamples(t *testing.T) {
	sample := map[string]bool{}
	for _, name := range []string{"collections-basic.json", "collections-extended.json"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "api", "samples", name))
		if err != nil {
			t.Fatalf("reading sample: %v", err)
		}
		jsonKeyPaths(t, raw, sample)
	}

	raw, err := json.Marshal([]PushCollection{{
		ID: "c", Title: "C", BackdropImageURL: "b", PinToTop: true, FocusGlowEnabled: true,
		ViewMode: "ROWS", ShowAllTab: true,
		Folders: []PushFolder{{
			ID: "f", Title: "F", CoverImageURL: "i", CoverEmoji: "e", FocusGIFURL: "g", FocusGIFEnabled: true,
			HeroBackdropURL: "h", HeroVideoURL: "v", TitleLogoURL: "l", TileShape: "POSTER", HideTitle: true,
			CatalogSources: []CatalogSource{{AddonID: "a", Type: "movie", CatalogID: "tmdb-x", Genre: "Action"}},
		}},
	}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	uno := map[string]bool{}
	jsonKeyPaths(t, raw, uno)

	for path := range uno {
		if !sample[path] {
			t.Errorf("Uno pushes %s, which neither sample carries: a renamed field?", path)
		}
	}
}
