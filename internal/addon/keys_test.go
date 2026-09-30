package addon

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/tmdbkey"
)

const ownerKey = "0123456789abcdef0123456789abcdef"

// perAccountServer is the addon routes on a server where each account brings
// its own TMDB key, with the owner's saved and the other profile's account
// keyless.
func (f handlerFixture) perAccountServer(t *testing.T) (*http.ServeMux, *Server) {
	t.Helper()
	box, err := tmdbkey.NewBox(bytes.Repeat([]byte{7}, tmdbkey.SecretSize))
	if err != nil {
		t.Fatal(err)
	}
	keys := tmdbkey.New(box, f.db)
	stored, err := keys.Seal("owner", ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetAccountKey(t.Context(), "owner", stored); err != nil {
		t.Fatal(err)
	}
	s, err := New(f.db, provider.NewTMDBClient(""), keys)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+ManifestPathPattern, s.Public(s.ManifestHandler))
	mux.HandleFunc("GET /u/{token}/catalog/{type}/{rest...}", s.Public(s.CatalogHandler))
	return mux, s
}

// keyRecorder is tmdbUp, recording the api_key of every request.
func keyRecorder(t *testing.T) func() []string {
	var mu sync.Mutex
	var keys []string
	fakeTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		keys = append(keys, r.URL.Query().Get("api_key"))
		mu.Unlock()
		tmdbUp(w, r)
	})
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), keys...)
	}
}

// On a per-account server each route reaches TMDB with the key of the
// account that owns the token's profile. A keyless owner's catalog fails
// without reaching TMDB, until another account has put the same page in the
// shared cache.
func TestAddonUsesTheOwnersKey(t *testing.T) {
	f := newHandlerFixture(t)
	keys := keyRecorder(t)
	mux, _ := f.perAccountServer(t)
	get := func(path string) int {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		return w.Code
	}
	theirs := "/u/" + f.other.Token + "/catalog/movie/" + ManifestID(f.theirs) + ".json"

	if code := get(theirs); code != http.StatusBadGateway || len(keys()) != 0 {
		t.Fatalf("a keyless owner's catalog = %d after %d TMDB calls; want 502 after none", code, len(keys()))
	}
	if code := get("/u/" + f.owner.Token + "/catalog/movie/" + ManifestID(f.onHome) + ".json"); code != http.StatusOK {
		t.Fatalf("the owner's catalog = %d, want 200", code)
	}
	if code := get("/u/" + f.owner.Token + "/manifest.json"); code != http.StatusOK {
		t.Fatalf("the owner's manifest = %d, want 200", code)
	}
	sent := keys()
	if len(sent) == 0 {
		t.Fatal("TMDB was never reached")
	}
	for _, k := range sent {
		if k != ownerKey {
			t.Errorf("a call went out with %q, want the owner's key", k)
		}
	}
	if code := get(theirs); code != http.StatusOK || len(keys()) != len(sent) {
		t.Errorf("the keyless owner's catalog once the page is cached = %d after %d more calls; want 200 from the cache", code, len(keys())-len(sent))
	}
}

// A problem with the owner's key is logged as that; TMDB failing is logged
// as it was.
func TestLogTMDBFailure(t *testing.T) {
	var out bytes.Buffer
	log.SetOutput(&out)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	logTMDBFailure("catalog x", fmt.Errorf("provider: fetch /d: %w", provider.ErrNoKey))
	logTMDBFailure("catalog y", errors.New("provider: TMDB returned status 503 for /d"))
	got := out.String()
	if !strings.Contains(got, "addon: catalog x: the profile owner's TMDB key can't be used: provider: fetch /d") {
		t.Errorf("a key problem logged as %q", got)
	}
	if !strings.Contains(got, "addon: catalog y: provider: TMDB returned status 503") || strings.Count(got, "can't be used") != 1 {
		t.Errorf("a TMDB failure logged as %q", got)
	}
}
