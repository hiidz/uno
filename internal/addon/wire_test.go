package addon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
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

// jsonKeyPaths is keyPaths over raw JSON.
func jsonKeyPaths(t *testing.T, raw []byte) map[string]bool {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decoding %s: %v", raw, err)
	}
	out := map[string]bool{}
	keyPaths(v, "$", out)
	return out
}

// sampleKeyPaths is jsonKeyPaths over one file in docs/api/samples, which
// is only ever read.
func sampleKeyPaths(t *testing.T, name string) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "api", "samples", name))
	if err != nil {
		t.Fatalf("reading sample: %v", err)
	}
	return jsonKeyPaths(t, raw)
}

// requireKnownKeys fails for each key path Uno emits that the sample lacks,
// unless it is one of unsampled, and for each unsampled path Uno no longer
// emits, so the exceptions can't outlive their reason.
func requireKnownKeys(t *testing.T, uno, sample map[string]bool, unsampled map[string]string) {
	t.Helper()
	for path := range uno {
		if !sample[path] && unsampled[path] == "" {
			t.Errorf("Uno emits %s, which the sample never carries: a renamed field?", path)
		}
	}
	for path := range unsampled {
		if !uno[path] {
			t.Errorf("exception %s is no longer emitted; drop it", path)
		}
	}
}

// TestManifestKeysMatchSample checks every key the manifest carries against
// docs/api/samples/manifest.json, a real Stremio manifest. Each catalog is
// built with every optional field set, so none is skipped as empty.
func TestManifestKeysMatchSample(t *testing.T) {
	selection := []vault.SelectedCatalog{
		selectedWithParams("{}", true),
		selectedWithParams("{}", false),
	}
	m := buildManifest(selection, actionComedy)
	m.Logo = "https://uno.example" + LogoPath
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	requireKnownKeys(t, jsonKeyPaths(t, raw), sampleKeyPaths(t, "manifest.json"), map[string]string{
		"$.catalogs[].showInHome": "Nuvio TV's own field, absent from Stremio manifests; see docs/architecture.md",
	})
}

// TestCatalogResponseKeysMatchSample checks every key a catalog page carries
// against docs/api/samples/catalog-response.json, a real Cinemeta response,
// with every optional Meta field set.
func TestCatalogResponseKeysMatchSample(t *testing.T) {
	raw, err := json.Marshal(catalogResponse{
		Metas: []provider.Meta{{
			ID: "tt0468569", Type: "movie", Name: "The Dark Knight",
			Poster: "p", Background: "b", Description: "d",
			ReleaseInfo: "2008", Released: "2008-07-16T00:00:00.000Z", Genres: []string{"Action"},
		}},
		CacheMaxAge:     catalogCacheMaxAge,
		StaleRevalidate: catalogStaleRevalidate,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	requireKnownKeys(t, jsonKeyPaths(t, raw), sampleKeyPaths(t, "catalog-response.json"), map[string]string{
		"$.metas[].released": "the full release date Stremio-protocol clients parse, which this Cinemeta sample lacks; see docs/architecture.md",
	})
}
