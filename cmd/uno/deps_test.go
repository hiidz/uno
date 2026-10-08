package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/hiidz/uno/internal/config"
	"github.com/hiidz/uno/internal/vault"
)

func testVault(t *testing.T) *vault.DB {
	t.Helper()
	db, err := vault.InitDB(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// A shared-key server has no per-account keys; a per-account one has them
// under its secret, and a secret of the wrong size stops the start.
func TestAPIDepsKeys(t *testing.T) {
	db := testVault(t)
	cfg := config.Config{TMDBAPIKey: "k", SiteBaseURL: "http://uno.example"}
	deps, err := apiDeps(cfg, db)
	if err != nil || deps.Keys != nil || deps.Provider == nil {
		t.Fatalf("shared: %+v, %v; want no keys", deps, err)
	}

	cfg = config.Config{PerAccountKeys: true, Secret: bytes.Repeat([]byte{1}, 32), SiteBaseURL: "http://uno.example"}
	if deps, err = apiDeps(cfg, db); err != nil || deps.Keys == nil {
		t.Fatalf("per-account: %+v, %v; want keys", deps, err)
	}

	cfg.Secret = []byte("short")
	if _, err := apiDeps(cfg, db); err == nil {
		t.Error("a short secret was accepted")
	}
}

// The dev auth bypass wraps the verifier and Nuvio client and admits its
// account through the access policy.
func TestAPIDepsDevBypass(t *testing.T) {
	cfg := config.Config{TMDBAPIKey: "k", SiteBaseURL: "http://uno.example", DevAuthBypassToken: "dev", Access: config.Access{Allowlist: true}}
	deps, err := apiDeps(cfg, testVault(t))
	if err != nil {
		t.Fatal(err)
	}
	if !deps.Access.Allowlist || !deps.Access.DevBypass {
		t.Errorf("access = %+v, want the bypass account admitted", deps.Access)
	}
	claims, err := deps.Verifier.Verify(t.Context(), "dev")
	if err != nil || claims.Sub == "" {
		t.Errorf("the bypass token = %+v, %v; want it verified", claims, err)
	}
}
