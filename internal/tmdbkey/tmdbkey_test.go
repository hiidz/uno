package tmdbkey

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

func testBox(t *testing.T, fill byte) *Box {
	t.Helper()
	b, err := NewBox(bytes.Repeat([]byte{fill}, SecretSize))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A sealed key opens for its account under the same secret, and for no
// other account, under no other secret, and not once tampered with.
func TestBoxRoundTrip(t *testing.T) {
	b := testBox(t, 1)
	sealed, err := b.Seal("acct", "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("0123456789abcdef")) {
		t.Fatal("the sealed key holds the key")
	}
	if key, err := b.Open("acct", sealed); err != nil || key != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("Open = %q, %v; want the key", key, err)
	}
	again, _ := b.Seal("acct", "0123456789abcdef0123456789abcdef")
	if bytes.Equal(again, sealed) {
		t.Error("sealing twice gave the same bytes; want a fresh nonce each time")
	}

	tampered := bytes.Clone(sealed)
	tampered[len(tampered)-1] ^= 1
	for name, open := range map[string]func() (string, error){
		"another account": func() (string, error) { return b.Open("other", sealed) },
		"another secret":  func() (string, error) { return testBox(t, 2).Open("acct", sealed) },
		"tampered":        func() (string, error) { return b.Open("acct", tampered) },
		"too short":       func() (string, error) { return b.Open("acct", sealed[:4]) },
	} {
		if key, err := open(); err == nil {
			t.Errorf("%s: opened as %q, want a failure", name, key)
		}
	}
}

func TestNewBoxWantsA32ByteSecret(t *testing.T) {
	for _, n := range []int{0, 16, 31, 33} {
		if _, err := NewBox(make([]byte, n)); err == nil {
			t.Errorf("a %d-byte secret was accepted", n)
		}
	}
}

// Clean takes a pasted API key, trimmed, and tells a Read Access Token from
// anything else that isn't one.
func TestClean(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		err      error
	}{
		{"0123456789abcdef0123456789ABCDEF", "0123456789abcdef0123456789ABCDEF", nil},
		{"  0123456789abcdef0123456789abcdef\n", "0123456789abcdef0123456789abcdef", nil},
		{"eyJhbGciOiJIUzI1NiJ9.eyJhdWQiOiJ4In0.sig", "", ErrReadAccessToken},
		{"0123456789abcdef0123456789abcde", "", ErrNotAPIKey},
		{"0123456789abcdef0123456789abcdeg", "", ErrNotAPIKey},
		{"", "", ErrNotAPIKey},
	} {
		got, err := Clean(tc.in)
		if got != tc.want || !errors.Is(err, tc.err) {
			t.Errorf("Clean(%q) = %q, %v; want %q, %v", tc.in, got, err, tc.want, tc.err)
		}
	}
}

func testVault(t *testing.T) *vault.DB {
	t.Helper()
	db, err := vault.InitDB(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// Each source yields the account's key once it has saved one, ErrNoKey
// before, and ErrKeyRejected for a key sealed under another secret; nil Keys
// give nil sources.
func TestKeysSources(t *testing.T) {
	ctx := context.Background()
	db := testVault(t)
	profile, err := db.ResolveOrCreateProfile(ctx, "acct", 1, "uuid-1")
	if err != nil {
		t.Fatal(err)
	}
	keys := New(testBox(t, 1), db)
	const key = "0123456789abcdef0123456789abcdef"

	for name, src := range map[string]provider.KeySource{
		"account": keys.ForAccount(ctx, "acct"),
		"token":   keys.ForToken(ctx, profile.Token),
		"sealed":  keys.Sealed("acct", nil),
	} {
		if _, err := src(); !errors.Is(err, provider.ErrNoKey) {
			t.Errorf("%s before a key: %v, want ErrNoKey", name, err)
		}
	}

	stored, err := keys.Seal("acct", key)
	if err != nil || stored.Last4 != "cdef" {
		t.Fatalf("Seal = %+v, %v; want last4 cdef", stored, err)
	}
	if err := db.SetAccountKey(ctx, "acct", stored); err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string]provider.KeySource{
		"account": keys.ForAccount(ctx, "acct"),
		"token":   keys.ForToken(ctx, profile.Token),
		"sealed":  keys.Sealed("acct", stored.Sealed),
	} {
		if got, err := src(); err != nil || got != key {
			t.Errorf("%s with a key = %q, %v; want the key", name, got, err)
		}
	}

	if _, err := keys.ForToken(ctx, "no-such-token")(); !errors.Is(err, vault.ErrProfileNotFound) {
		t.Errorf("unknown token: %v, want ErrProfileNotFound", err)
	}
	rekeyed := New(testBox(t, 2), db)
	if _, err := rekeyed.ForAccount(ctx, "acct")(); !errors.Is(err, provider.ErrKeyRejected) || strings.Contains(err.Error(), key) {
		t.Errorf("under another secret: %v, want ErrKeyRejected without the key", err)
	}

	var shared *Keys
	if shared.ForAccount(ctx, "acct") != nil || shared.ForToken(ctx, "t") != nil || shared.Sealed("acct", nil) != nil {
		t.Error("nil Keys gave a source; want nil, the shared key's")
	}
}
