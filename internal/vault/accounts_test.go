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

	if _, err := db.AccountKey(ctx, "acct"); !errors.Is(err, ErrNoAccountKey) {
		t.Fatalf("before any key: %v, want ErrNoAccountKey", err)
	}
	if err := db.SetAccountKey(ctx, "acct", AccountKey{Sealed: []byte{1, 2}, Last4: "aaaa"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAccountKey(ctx, "acct", AccountKey{Sealed: []byte{3, 4}, Last4: "bbbb"}); err != nil {
		t.Fatalf("replacing: %v", err)
	}
	if err := db.SetAccountKey(ctx, "other", AccountKey{Sealed: []byte{5}, Last4: "cccc"}); err != nil {
		t.Fatal(err)
	}
	got, err := db.AccountKey(ctx, "acct")
	if err != nil || !bytes.Equal(got.Sealed, []byte{3, 4}) || got.Last4 != "bbbb" {
		t.Fatalf("AccountKey = %+v, %v; want the replacement", got, err)
	}

	if err := db.DeleteAccountKey(ctx, "acct"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteAccountKey(ctx, "acct"); err != nil {
		t.Fatalf("removing it again: %v", err)
	}
	if _, err := db.AccountKey(ctx, "acct"); !errors.Is(err, ErrNoAccountKey) {
		t.Errorf("after removing: %v, want ErrNoAccountKey", err)
	}
	if got, err := db.AccountKey(ctx, "other"); err != nil || got.Last4 != "cccc" {
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
		CatalogSelectionForm{Catalogs: []SelectedCatalogInput{{CatalogID: catalog.ID, ShowInHome: true}}},
		CollectionSelectionForm{})

	account, sealed, err := db.AccountKeyByToken(ctx, token)
	if err != nil || account != "owner" || sealed != nil {
		t.Fatalf("with no key = %q, %v, %v; want the account and no key", account, sealed, err)
	}
	if served, err := db.ServedCatalog(ctx, token, catalog.ID, "movie", "tmdb"); err != nil || served.Account != "owner" || served.SealedKey != nil {
		t.Fatalf("served with no key = %+v, %v; want the account and no key", served, err)
	}

	if err := db.SetAccountKey(ctx, "owner", AccountKey{Sealed: []byte{9}, Last4: "zzzz"}); err != nil {
		t.Fatal(err)
	}
	if account, sealed, err = db.AccountKeyByToken(ctx, token); err != nil || account != "owner" || !bytes.Equal(sealed, []byte{9}) {
		t.Errorf("with a key = %q, %v, %v; want the account and its key", account, sealed, err)
	}
	if served, err := db.ServedCatalog(ctx, token, catalog.ID, "movie", "tmdb"); err != nil || served.Params != "{}" || !bytes.Equal(served.SealedKey, []byte{9}) {
		t.Errorf("served with a key = %+v, %v; want the params and the key", served, err)
	}
	if _, _, err := db.AccountKeyByToken(ctx, "no-such-token"); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("unknown token: %v, want ErrProfileNotFound", err)
	}
}
