package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/vault"
)

// Each command parses from its own arguments; anything else is the usage.
func TestParseCommand(t *testing.T) {
	id := uuid.NewString()
	good := map[string]command{
		"takedown " + id:              {name: "takedown", publication: uuid.MustParse(id)},
		"takedown " + id + " --block": {name: "takedown", publication: uuid.MustParse(id), block: true},
		"unblock acct-1":              {name: "unblock", account: "acct-1"},
		"blocked":                     {name: "blocked"},
	}
	for line, want := range good {
		if got, err := parseCommand(strings.Fields(line)); err != nil || got != want {
			t.Errorf("parseCommand(%q) = %+v, %v; want %+v", line, got, err, want)
		}
	}
	for _, line := range []string{
		"serve", "takedown", "takedown nope", "takedown " + id + " --blok", "takedown --block " + id,
		"unblock", "unblock a b", "blocked now",
	} {
		if _, err := parseCommand(strings.Fields(line)); !errors.Is(err, errUsage) {
			t.Errorf("parseCommand(%q) = %v, want the usage", line, err)
		}
	}
}

// publishTestCatalog publishes a catalog named name from slot of account,
// returning its publication's id.
func publishTestCatalog(t *testing.T, db *vault.DB, account string, slot int, name string) uuid.UUID {
	t.Helper()
	p, err := db.ResolveOrCreateProfile(t.Context(), account, slot, account+"-profile")
	if err != nil {
		t.Fatal(err)
	}
	c, err := db.CreateUserCatalog(t.Context(), p.ID, vault.CatalogForm{Type: "movie", Name: name, Provider: "tmdb", Params: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	if c, err = db.PublishCatalog(t.Context(), p.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	return c.Publication.ID
}

// runTakedown runs a takedown of publication answering answer, returning
// what it wrote.
func runTakedown(t *testing.T, db *vault.DB, publication uuid.UUID, block bool, answer string) (string, error) {
	t.Helper()
	var out strings.Builder
	err := takedown(t.Context(), db, publication, block, strings.NewReader(answer), &out)
	return out.String(), err
}

// A takedown prints what it removes and the account, and acts only on y.
func TestTakedownAsksFirst(t *testing.T) {
	db := testVault(t)
	id := publishTestCatalog(t, db, "acct-1", 1, "Popular")

	for _, answer := range []string{"", "n\n", "yes please\n"} {
		out, err := runTakedown(t, db, id, false, answer)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`catalog "Popular"`, id.String(), "0 subscribers", "Published by account acct-1", "[y/N]", "Nothing changed."} {
			if !strings.Contains(out, want) {
				t.Errorf("answer %q: output %q lacks %q", answer, out, want)
			}
		}
	}
	if _, err := db.PreviewTakedown(t.Context(), id); err != nil {
		t.Fatalf("the post after declining = %v, want it still there", err)
	}

	out, err := runTakedown(t, db, id, false, "Y\n")
	if err != nil || !strings.Contains(out, "Took down:") || strings.Contains(out, "Blocked") {
		t.Fatalf("takedown = %q, %v; want it taken down, no block", out, err)
	}
	if _, err := db.PreviewTakedown(t.Context(), id); !errors.Is(err, vault.ErrPublicationNotFound) {
		t.Errorf("the post after the takedown = %v, want it gone", err)
	}
	if _, err := runTakedown(t, db, id, false, "y\n"); err == nil || !strings.Contains(err.Error(), "no publication") {
		t.Errorf("a second takedown = %v, want no publication", err)
	}
}

// --block lists every post of the account, across its profiles, takes them
// all down and blocks it; blocked lists the block and unblock lifts it.
func TestTakedownBlockAndUnblock(t *testing.T) {
	db := testVault(t)
	id := publishTestCatalog(t, db, "acct-1", 1, "Popular")
	publishTestCatalog(t, db, "acct-1", 2, "Other slot")
	kept := publishTestCatalog(t, db, "acct-2", 1, "Someone else")

	out, err := runTakedown(t, db, id, true, "y\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"Popular"`, `"Other slot"`, "Blocks account acct-1", "Blocked account acct-1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q lacks %q", out, want)
		}
	}
	if strings.Contains(out, "Someone else") {
		t.Errorf("output %q names another account's post", out)
	}
	if _, err := db.PreviewTakedown(t.Context(), kept); err != nil {
		t.Errorf("another account's post = %v, want it kept", err)
	}

	var list strings.Builder
	if err := listBlocked(t.Context(), db, &list); err != nil || !strings.Contains(list.String(), `acct-1`) || !strings.Contains(list.String(), `"Popular"`) {
		t.Errorf("blocked = %q, %v; want acct-1 from Popular", list.String(), err)
	}

	for _, want := range []string{"Unblocked account acct-1", "isn't blocked"} {
		var o strings.Builder
		if err := unblock(t.Context(), db, "acct-1", &o); err != nil || !strings.Contains(o.String(), want) {
			t.Errorf("unblock = %q, %v; want %q", o.String(), err, want)
		}
	}
	list.Reset()
	if err := listBlocked(t.Context(), db, &list); err != nil || list.String() != "No account is blocked.\n" {
		t.Errorf("blocked after unblocking = %q, %v", list.String(), err)
	}
}
