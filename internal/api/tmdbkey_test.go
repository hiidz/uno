package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/tmdbkey"
	"github.com/hiidz/uno/internal/vault"
)

const goodKey = "0123456789abcdef0123456789abcdef"

// newPerAccountServer is a server where each account brings its own TMDB key:
// no shared key, and keys sealed under a fixed test secret.
func newPerAccountServer(t *testing.T, db *vault.DB) *Server {
	t.Helper()
	box, err := tmdbkey.NewBox(bytes.Repeat([]byte{9}, tmdbkey.SecretSize))
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(Deps{
		Vault:        db,
		Provider:     provider.NewTMDBClient(""),
		Verifier:     acceptAnyToken{},
		Nuvio:        &fakeNuvio{},
		SiteBaseURL:  "http://example.com",
		NuvioBaseURL: "https://nuvio.example.com",
		Keys:         tmdbkey.New(box, db),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// keyedTMDB is tmdbUp for goodKey, a 401 for any other key, answering
// /authentication as TMDB does, and records every api_key it is sent.
func keyedTMDB(t *testing.T) func() []string {
	t.Helper()
	var mu sync.Mutex
	var keys []string
	fakeTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("api_key")
		mu.Lock()
		keys = append(keys, key)
		mu.Unlock()
		switch {
		case key != goodKey:
			http.Error(w, `{"status_code":7}`, http.StatusUnauthorized)
		case r.URL.Path == "/3/authentication":
			w.Write([]byte(`{"success":true}`))
		default:
			tmdbUp(w, r)
		}
	})
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), keys...)
	}
}

// The config route says the mode, and needs no sign-in.
func TestConfigRoute(t *testing.T) {
	db := newTestVaultDB(t)
	for _, tc := range []struct {
		s    *Server
		want string
	}{{newProfileTestServer(t, db), "shared"}, {newPerAccountServer(t, db), "per-account"}} {
		w := serve(t, tc.s, http.MethodGet, "/api/config", "", true)
		if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"tmdb_key_mode":"`+tc.want+`"}` {
			t.Errorf("GET /api/config = %d %s, want %s", w.Code, w.Body.String(), tc.want)
		}
	}
}

// On a server with a shared key the key routes don't exist.
func TestTMDBKeyRoutesAreAbsentInSharedMode(t *testing.T) {
	noTMDB(t)
	s := newProfileTestServer(t, newTestVaultDB(t))
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		if w := serve(t, s, method, "/api/account/tmdb-key", `{"key":"`+goodKey+`"}`, false); w.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", method, w.Code)
		}
	}
}

// An account saves, replaces and removes its key; a key of the wrong shape,
// one TMDB refuses, and one TMDB couldn't check are each refused with nothing
// saved. The key itself is never answered or stored in the clear.
func TestTMDBKeyRoutes(t *testing.T) {
	db := newTestVaultDB(t)
	s := newPerAccountServer(t, db)
	sent := keyedTMDB(t)

	status := func() string {
		t.Helper()
		w := serve(t, s, http.MethodGet, "/api/account/tmdb-key", "", false)
		if w.Code != http.StatusOK {
			t.Fatalf("GET = %d %s", w.Code, w.Body.String())
		}
		return strings.TrimSpace(w.Body.String())
	}
	if got := status(); got != `{"set":false}` {
		t.Fatalf("before any key = %s", got)
	}

	for _, tc := range []struct {
		name, body string
		code       int
		want       string
	}{
		{"not a key", `{"key":"hello"}`, http.StatusBadRequest, "32 characters, 0–9 and a–f"},
		{"a Read Access Token", `{"key":"eyJhbGciOiJIUzI1NiJ9.e30.x"}`, http.StatusBadRequest, "Read Access Token"},
		{"refused by TMDB", `{"key":"ffffffffffffffffffffffffffffffff"}`, http.StatusBadRequest, "TMDB didn't accept this key"},
		{"a bad body", `{`, http.StatusBadRequest, "invalid request body"},
		{"a field the body doesn't have", `{"key":"ffffffffffffffffffffffffffffffff","api_key":"x"}`, http.StatusBadRequest, `unknown field "api_key"`},
	} {
		w := serve(t, s, http.MethodPut, "/api/account/tmdb-key", tc.body, false)
		if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.want) {
			t.Errorf("%s: %d %q, want %d containing %q", tc.name, w.Code, w.Body.String(), tc.code, tc.want)
		}
	}
	if got := status(); got != `{"set":false}` {
		t.Fatalf("after the refusals = %s, want nothing saved", got)
	}
	if got := strings.Join(sent(), ","); got != "ffffffffffffffffffffffffffffffff" {
		t.Errorf("keys sent to TMDB = %s, want only the well-formed one", got)
	}

	w := serve(t, s, http.MethodPut, "/api/account/tmdb-key", `{"key":"  `+goodKey+` "}`, false)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"set":true,"last4":"cdef"}` {
		t.Fatalf("PUT a good key = %d %s", w.Code, w.Body.String())
	}
	if got := status(); got != `{"set":true,"last4":"cdef"}` {
		t.Errorf("after saving = %s", got)
	}
	stored, err := db.AccountKey(t.Context(), "test-sub")
	if err != nil || bytes.Contains(stored.Sealed, []byte(goodKey)) {
		t.Errorf("stored = %v, %v; want it sealed", stored, err)
	}

	if w := serve(t, s, http.MethodDelete, "/api/account/tmdb-key", "", false); w.Code != http.StatusNoContent {
		t.Errorf("DELETE = %d", w.Code)
	}
	if got := status(); got != `{"set":false}` {
		t.Errorf("after removing = %s", got)
	}
}

// A key TMDB can't be reached to check is a 502 and saves nothing.
func TestTMDBKeyCheckWhileTMDBIsDown(t *testing.T) {
	db := newTestVaultDB(t)
	s := newPerAccountServer(t, db)
	fakeTMDB(t, tmdbDown)
	w := serve(t, s, http.MethodPut, "/api/account/tmdb-key", `{"key":"`+goodKey+`"}`, false)
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "Nothing was saved") {
		t.Errorf("PUT while TMDB is down = %d %q", w.Code, w.Body.String())
	}
	if _, err := db.AccountKey(t.Context(), "test-sub"); err == nil {
		t.Error("a key was saved")
	}
}

// The builder reaches TMDB with the signed-in account's own key. Without one,
// and with one TMDB refuses, every route that reaches TMDB answers 422 in
// words; a route that doesn't reach TMDB never reads the key.
func TestBuilderUsesTheAccountsKey(t *testing.T) {
	db := newTestVaultDB(t)
	if _, err := db.ResolveOrCreateProfile(t.Context(), "test-sub", 1, "nuvio-profile"); err != nil {
		t.Fatal(err)
	}
	sent := keyedTMDB(t)
	preview := `{"type":"movie","params":"{\"sort_by\":\"popularity.desc\"}"}`
	create := `{"type":"movie","name":"New","provider":"tmdb","params":"{\"with_genres\":\"28\"}"}`
	routes := []struct{ method, path, body string }{
		{http.MethodPost, "/api/catalogs/preview", preview},
		{http.MethodGet, "/api/genres/movie", ""},
		{http.MethodPost, "/api/p/1/catalogs", create},
	}
	check := func(s *Server, code int, want string) {
		t.Helper()
		for _, r := range routes {
			w := serve(t, s, r.method, r.path, r.body, false)
			if w.Code != code || !strings.Contains(w.Body.String(), want) {
				t.Errorf("%s %s = %d %q, want %d containing %q", r.method, r.path, w.Code, w.Body.String(), code, want)
			}
		}
	}

	check(newPerAccountServer(t, db), http.StatusUnprocessableEntity, "no TMDB key")
	if len(sent()) != 0 {
		t.Fatalf("TMDB was reached %d times without a key", len(sent()))
	}
	if w := serve(t, newPerAccountServer(t, db), http.MethodGet, "/api/p/1/catalogs", "", false); w.Code != http.StatusOK {
		t.Errorf("a list with no key = %d, want 200", w.Code)
	}

	s := newPerAccountServer(t, db)
	if w := serve(t, s, http.MethodPut, "/api/account/tmdb-key", `{"key":"`+goodKey+`"}`, false); w.Code != http.StatusOK {
		t.Fatalf("PUT = %d", w.Code)
	}
	for i, r := range routes {
		w := serve(t, s, r.method, r.path, r.body, false)
		if w.Code >= 300 {
			t.Errorf("route %d with a key = %d %s", i, w.Code, w.Body.String())
		}
	}
	for _, k := range sent() {
		if k != goodKey {
			t.Errorf("a call went out with %q, want the account's key", k)
		}
	}

	stored, err := newPerAccountServer(t, db).keys.Seal("test-sub", "ffffffffffffffffffffffffffffffff")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetAccountKey(t.Context(), "test-sub", stored); err != nil {
		t.Fatal(err)
	}
	check(newPerAccountServer(t, db), http.StatusUnprocessableEntity, "TMDB didn't accept your key")
}

// A 401 on the server's shared key is TMDB failing, the operator's to fix: a
// 502, never the account's 422.
func TestSharedKeyRejectedIsUpstream(t *testing.T) {
	fakeTMDB(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "{}", http.StatusUnauthorized)
	})
	s := newProfileTestServer(t, newTestVaultDB(t))
	if w := serve(t, s, http.MethodGet, "/api/genres/movie", "", false); w.Code != http.StatusBadGateway {
		t.Errorf("GET /api/genres/movie with a refused shared key = %d, want 502", w.Code)
	}
	w := serve(t, s, http.MethodPost, "/api/catalogs/preview", `{"type":"movie","params":"{}"}`, false)
	var body map[string]any
	if w.Code != http.StatusBadGateway || json.Unmarshal(w.Body.Bytes(), &body) == nil {
		t.Errorf("preview with a refused shared key = %d %q, want a plain 502", w.Code, w.Body.String())
	}
}
