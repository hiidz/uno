package main

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hiidz/uno/internal/vault"
)

//go:embed testdata/schema_v9.sql
var schemaV9 string

// newV9Database is a v9 database at a new path, made from the v9 schema.
func newV9Database(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vault.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.Exec(schemaV9 + "\nPRAGMA user_version = 9;"); err != nil {
		t.Fatal(err)
	}
	return path
}

// A v9 database migrates to one this build opens, with both Community
// indexes; a second run is refused.
func TestMigrateToV10(t *testing.T) {
	ctx := context.Background()
	path := newV9Database(t)
	var out bytes.Buffer
	if err := runCommand(ctx, []string{"migrate", "--db", path}, &out); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(out.String(), "schema version 10") {
		t.Errorf("output = %q, want it to name version 10", out.String())
	}
	db, err := vault.InitDB(path)
	if err != nil {
		t.Fatalf("InitDB after migrating: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runMigrate(ctx, []string{"--db", path}, &out); err == nil || !strings.Contains(err.Error(), "version 10") {
		t.Errorf("a second migrate = %v, want it refused at version 10", err)
	}
}

// migrate needs --db naming a file that is there.
func TestMigrateRefusesItsArguments(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	if err := runMigrate(ctx, nil, &out); err == nil {
		t.Error("migrate with no --db succeeded")
	}
	if err := runMigrate(ctx, []string{"--db", filepath.Join(t.TempDir(), "none.db")}, &out); err == nil {
		t.Error("migrate of a missing file succeeded")
	}
}
