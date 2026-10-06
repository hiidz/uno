package vault

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// Two different Nuvio accounts each populate profile slot 1. Lookups are
// scoped by (nuvio_user_id, nuvio_profile_index) together, so one account
// must never resolve, fetch, or otherwise reach the other's profile at that
// shared slot number.
func TestCrossAccountIsolationAtSameSlot(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	const (
		accountA = "nuvio-user-a"
		accountB = "nuvio-user-b"
		slot     = 1
	)

	profA, err := db.ResolveOrCreateProfile(ctx, accountA, slot, "nuvio-profile-uuid-a")
	if err != nil {
		t.Fatalf("creating account A's profile: %v", err)
	}
	profB, err := db.ResolveOrCreateProfile(ctx, accountB, slot, "nuvio-profile-uuid-b")
	if err != nil {
		t.Fatalf("creating account B's profile: %v", err)
	}

	if profA.ID == profB.ID {
		t.Fatalf("accounts A and B resolved to the same profile row at slot %d: %s", slot, profA.ID)
	}
	if profA.Token == profB.Token {
		t.Fatalf("accounts A and B share a profile token: %q", profA.Token)
	}

	// GetProfileBySlot must return each account's own row, not the other's.
	gotA, err := db.GetProfileBySlot(ctx, accountA, slot)
	if err != nil {
		t.Fatalf("GetProfileBySlot(A): %v", err)
	}
	if gotA.ID != profA.ID {
		t.Fatalf("GetProfileBySlot(A) = %s, want %s", gotA.ID, profA.ID)
	}

	gotB, err := db.GetProfileBySlot(ctx, accountB, slot)
	if err != nil {
		t.Fatalf("GetProfileBySlot(B): %v", err)
	}
	if gotB.ID != profB.ID {
		t.Fatalf("GetProfileBySlot(B) = %s, want %s", gotB.ID, profB.ID)
	}

	// Re-resolving with account A's ID must never hand back account B's row
	// (or vice versa), and must not perturb B's stored Nuvio profile UUID.
	reResolvedA, err := db.ResolveOrCreateProfile(ctx, accountA, slot, "nuvio-profile-uuid-a")
	if err != nil {
		t.Fatalf("re-resolving account A's profile: %v", err)
	}
	if reResolvedA.ID != profA.ID {
		t.Fatalf("re-resolving account A returned a different profile: got %s, want %s", reResolvedA.ID, profA.ID)
	}

	fetchedB, err := db.GetProfileBySlot(ctx, accountB, slot)
	if err != nil {
		t.Fatalf("GetProfileBySlot(B): %v", err)
	}
	if fetchedB.NuvioProfileUUID != "nuvio-profile-uuid-b" {
		t.Fatalf("account B's profile UUID was perturbed by account A's activity: got %q", fetchedB.NuvioProfileUUID)
	}
	if fetchedB.NuvioUserID != accountB {
		t.Fatalf("GetProfileBySlot(B) returned a profile owned by %q, want %q", fetchedB.NuvioUserID, accountB)
	}

	// A's bearer token must resolve only to A's profile, never B's.
	resolvedID, err := db.ResolveProfileID(ctx, profA.Token)
	if err != nil {
		t.Fatalf("ResolveProfileID(A's token): %v", err)
	}
	if resolvedID != profA.ID {
		t.Fatalf("ResolveProfileID(A's token) = %s, want %s", resolvedID, profA.ID)
	}

	// B's token must not resolve to A's profile either.
	resolvedID, err = db.ResolveProfileID(ctx, profB.Token)
	if err != nil {
		t.Fatalf("ResolveProfileID(B's token): %v", err)
	}
	if resolvedID != profB.ID {
		t.Fatalf("ResolveProfileID(B's token) = %s, want %s", resolvedID, profB.ID)
	}

	// A token from an unrelated account/slot must not resolve at all.
	if _, err := db.ResolveProfileID(ctx, "not-a-real-token"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("ResolveProfileID(bogus token) error = %v, want ErrProfileNotFound", err)
	}
}

// A Nuvio profile deleted and recreated at the same slot comes back with a
// new UUID. The slot keeps its Uno profile, and with it the token every
// installed manifest URL carries; only the stored UUID follows Nuvio.
func TestResolveOrCreateProfileFollowsNuvioProfileDrift(t *testing.T) {
	ctx := t.Context()
	db := newTestDB(t)

	first, err := db.ResolveOrCreateProfile(ctx, "nuvio-user", 2, "nuvio-profile-uuid-old")
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}
	again, err := db.ResolveOrCreateProfile(ctx, "nuvio-user", 2, "nuvio-profile-uuid-old")
	if err != nil {
		t.Fatalf("resolving unchanged profile: %v", err)
	}
	if again != first {
		t.Fatalf("resolving an unchanged slot = %+v, want %+v", again, first)
	}

	drifted, err := db.ResolveOrCreateProfile(ctx, "nuvio-user", 2, "nuvio-profile-uuid-new")
	if err != nil {
		t.Fatalf("resolving drifted profile: %v", err)
	}
	if drifted.ID != first.ID || drifted.Token != first.Token {
		t.Fatalf("drifted profile = %+v, want the same id and token as %+v", drifted, first)
	}
	if drifted.NuvioProfileUUID != "nuvio-profile-uuid-new" {
		t.Fatalf("returned NuvioProfileUUID = %q, want the new one", drifted.NuvioProfileUUID)
	}
	stored, err := db.GetProfileBySlot(ctx, "nuvio-user", 2)
	if err != nil {
		t.Fatalf("GetProfileBySlot: %v", err)
	}
	if stored.NuvioProfileUUID != "nuvio-profile-uuid-new" {
		t.Fatalf("stored NuvioProfileUUID = %q, want the new one", stored.NuvioProfileUUID)
	}
}
