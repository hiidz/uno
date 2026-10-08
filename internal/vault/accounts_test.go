package vault

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

// An account's key is stored, replaced, read and removed, and one account's
// key is never another's.
func TestAccountKey(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if _, err := db.AccountKey(ctx, "acct", "tmdb"); !errors.Is(err, ErrNoAccountKey) {
		t.Fatalf("before any key: %v, want ErrNoAccountKey", err)
	}
	if err := db.SetAccountKey(ctx, "acct", "tmdb", AccountKey{Sealed: []byte{1, 2}, Last4: "aaaa"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAccountKey(ctx, "acct", "tmdb", AccountKey{Sealed: []byte{3, 4}, Last4: "bbbb"}); err != nil {
		t.Fatalf("replacing: %v", err)
	}
	if err := db.SetAccountKey(ctx, "other", "tmdb", AccountKey{Sealed: []byte{5}, Last4: "cccc"}); err != nil {
		t.Fatal(err)
	}
	got, err := db.AccountKey(ctx, "acct", "tmdb")
	if err != nil || !bytes.Equal(got.Sealed, []byte{3, 4}) || got.Last4 != "bbbb" {
		t.Fatalf("AccountKey = %+v, %v; want the replacement", got, err)
	}

	if err := db.DeleteAccountKey(ctx, "acct", "tmdb"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteAccountKey(ctx, "acct", "tmdb"); err != nil {
		t.Fatalf("removing it again: %v", err)
	}
	if _, err := db.AccountKey(ctx, "acct", "tmdb"); !errors.Is(err, ErrNoAccountKey) {
		t.Errorf("after removing: %v, want ErrNoAccountKey", err)
	}
	if got, err := db.AccountKey(ctx, "other", "tmdb"); err != nil || got.Last4 != "cccc" {
		t.Errorf("the other account's key = %+v, %v; want it kept", got, err)
	}
}

// A profile's token leads to its account and that account's sealed key, nil
// while it has none; so does the one addon catalog lookup.
func TestAccountKeyByToken(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	profile := newTestProfile(t, db, "owner")
	token := profileToken(t, db, profile)
	catalog, err := db.CreateUserCatalog(ctx, profile, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatal(err)
	}
	savePush(t, db, profile,
		PushedHome{Catalogs: []SelectedCatalogInput{{CatalogID: catalog.ID, ShowInHome: true}}})

	account, sealed, err := db.AccountKeyByToken(ctx, token, "tmdb")
	if err != nil || account != "owner" || sealed != nil {
		t.Fatalf("with no key = %q, %v, %v; want the account and no key", account, sealed, err)
	}
	if served, err := db.ServedCatalog(ctx, token, catalog.ID, "movie", "tmdb"); err != nil || served.Account != "owner" || served.SealedKey != nil {
		t.Fatalf("served with no key = %+v, %v; want the account and no key", served, err)
	}

	if err := db.SetAccountKey(ctx, "owner", "tmdb", AccountKey{Sealed: []byte{9}, Last4: "zzzz"}); err != nil {
		t.Fatal(err)
	}
	if account, sealed, err = db.AccountKeyByToken(ctx, token, "tmdb"); err != nil || account != "owner" || !bytes.Equal(sealed, []byte{9}) {
		t.Errorf("with a key = %q, %v, %v; want the account and its key", account, sealed, err)
	}
	if served, err := db.ServedCatalog(ctx, token, catalog.ID, "movie", "tmdb"); err != nil || served.Params != "{}" || !bytes.Equal(served.SealedKey, []byte{9}) {
		t.Errorf("served with a key = %+v, %v; want the params and the key", served, err)
	}
	if _, _, err := db.AccountKeyByToken(ctx, "no-such-token", "tmdb"); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("unknown token: %v, want ErrProfileNotFound", err)
	}
}

// An account's keys for two providers are two rows: each read, served and
// removed by its own provider, and neither fanning the token's join out.
func TestAccountKeyPerProvider(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	profile := newTestProfile(t, db, "owner")
	token := profileToken(t, db, profile)
	catalog, err := db.CreateUserCatalog(ctx, profile, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatal(err)
	}
	savePush(t, db, profile,
		PushedHome{Catalogs: []SelectedCatalogInput{{CatalogID: catalog.ID, ShowInHome: true}}})

	if err := db.SetAccountKey(ctx, "owner", "imdb", AccountKey{Sealed: []byte{7}, Last4: "iiii"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AccountKey(ctx, "owner", "tmdb"); !errors.Is(err, ErrNoAccountKey) {
		t.Errorf("tmdb with only an imdb key: %v, want ErrNoAccountKey", err)
	}
	if served, err := db.ServedCatalog(ctx, token, catalog.ID, "movie", "tmdb"); err != nil || served.SealedKey != nil || served.Provider != "tmdb" {
		t.Errorf("served with only an imdb key = %+v, %v; want no key", served, err)
	}

	if err := db.SetAccountKey(ctx, "owner", "tmdb", AccountKey{Sealed: []byte{9}, Last4: "tttt"}); err != nil {
		t.Fatal(err)
	}
	for provider, want := range map[string][]byte{"tmdb": {9}, "imdb": {7}} {
		if _, sealed, err := db.AccountKeyByToken(ctx, token, provider); err != nil || !bytes.Equal(sealed, want) {
			t.Errorf("AccountKeyByToken %s = %v, %v; want %v", provider, sealed, err, want)
		}
	}
	if served, err := db.ServedCatalog(ctx, token, catalog.ID, "movie", "tmdb"); err != nil || !bytes.Equal(served.SealedKey, []byte{9}) {
		t.Errorf("served with both keys = %+v, %v; want the tmdb key", served, err)
	}

	if err := db.DeleteAccountKey(ctx, "owner", "tmdb"); err != nil {
		t.Fatal(err)
	}
	if got, err := db.AccountKey(ctx, "owner", "imdb"); err != nil || got.Last4 != "iiii" {
		t.Errorf("the imdb key after removing tmdb's = %+v, %v; want it kept", got, err)
	}
}
