package vault

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// newSecondProfile makes profile slot 2 of account nuvioUserID.
func newSecondProfile(t *testing.T, db *DB, nuvioUserID string) uuid.UUID {
	t.Helper()
	p, err := db.ResolveOrCreateProfile(context.Background(), nuvioUserID, 2, "nuvio-profile-2-"+nuvioUserID)
	if err != nil {
		t.Fatalf("creating profile 2 of %s: %v", nuvioUserID, err)
	}
	return p.ID
}

// postIDs is the id of each of posts.
func postIDs(posts []Post) map[uuid.UUID]bool {
	ids := map[uuid.UUID]bool{}
	for _, p := range posts {
		ids[p.ID] = true
	}
	return ids
}

// A takedown deletes the one publication, whoever published it: Community
// stops listing it, the subscriber's copy stays as their own, and the
// publisher's source stays, not published.
func TestTakeDownFreesCopies(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber, stranger := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber"), newTestProfile(t, db, "stranger")
	c := publishCatalog(t, db, owner, "Popular", "{}")
	kept := publishCatalog(t, db, owner, "Kept", "{}")
	copied := subscribe(t, db, subscriber, c.Publication.ID)

	preview, err := db.PreviewTakedown(ctx, c.Publication.ID)
	if err != nil {
		t.Fatalf("PreviewTakedown: %v", err)
	}
	if preview.Post.Title != "Popular" || preview.Post.Kind != kindCatalog || preview.Post.SubscriberCount != 1 || preview.Account != "owner" || len(preview.AccountPosts) != 2 {
		t.Errorf("preview = %+v, want Popular by owner with 1 subscriber, 2 account posts", preview)
	}

	removed, err := db.TakeDown(ctx, c.Publication.ID, false)
	if err != nil {
		t.Fatalf("TakeDown: %v", err)
	}
	if len(removed) != 1 || removed[0].ID != c.Publication.ID {
		t.Errorf("removed = %+v, want just %s", removed, c.Publication.ID)
	}
	if items := listCommunity(t, db, stranger); len(items) != 1 || items[0].ID != kept.Publication.ID {
		t.Errorf("Community after the takedown = %+v, want just Kept", items)
	}
	if got := reloadCatalog(t, db, copied.Catalog.ID); got.Subscription != nil || got.Name != "Popular" {
		t.Errorf("subscriber's copy = %+v, want Popular, its subscriber's own", got)
	}
	if got := reloadCatalog(t, db, c.ID); got.Publication != nil {
		t.Errorf("publisher's catalog's publication = %+v, want none", got.Publication)
	}
	if _, err := db.PublishCatalog(ctx, owner, c.ID); err != nil {
		t.Errorf("publish again after a takedown without block = %v", err)
	}
}

// A takedown of an unknown publication is not found, and so is its preview.
func TestTakeDownUnknownPublication(t *testing.T) {
	db := newTestDB(t)
	id := uuid.New()
	if _, err := db.PreviewTakedown(context.Background(), id); !errors.Is(err, ErrPublicationNotFound) {
		t.Errorf("PreviewTakedown = %v, want ErrPublicationNotFound", err)
	}
	if _, err := db.TakeDown(context.Background(), id, true); !errors.Is(err, ErrPublicationNotFound) {
		t.Errorf("TakeDown = %v, want ErrPublicationNotFound", err)
	}
	if blocks, err := db.Blocked(context.Background()); err != nil || len(blocks) != 0 {
		t.Errorf("Blocked = %+v, %v; want none", blocks, err)
	}
}

// A takedown with block removes every post of every profile of the account,
// and no other account's. Every profile of the account is refused a publish
// of a catalog or a collection until it is unblocked; Blocked lists the
// block with the post's title until then.
func TestTakeDownBlocksTheAccount(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	slot1, other, stranger := newTestProfile(t, db, "owner"), newTestProfile(t, db, "other"), newTestProfile(t, db, "stranger")
	slot2 := newSecondProfile(t, db, "owner")
	c := publishCatalog(t, db, slot1, "Popular", "{}")
	coll := publishCollection(t, db, slot2, CollectionForm{Title: "Weekend", ViewMode: "TABBED_GRID"})
	others := publishCatalog(t, db, other, "Theirs", "{}")

	removed, err := db.TakeDown(ctx, c.Publication.ID, true)
	if err != nil {
		t.Fatalf("TakeDown: %v", err)
	}
	if ids := postIDs(removed); len(ids) != 2 || !ids[c.Publication.ID] || !ids[coll.Publication.ID] {
		t.Errorf("removed = %+v, want both of owner's posts", removed)
	}
	if items := listCommunity(t, db, stranger); len(items) != 1 || items[0].ID != others.Publication.ID {
		t.Errorf("Community after the block = %+v, want just Theirs", items)
	}

	blocks, err := db.Blocked(ctx)
	if err != nil || len(blocks) != 1 || blocks[0].Account != "owner" || blocks[0].Title != "Popular" || blocks[0].BlockedAt.IsZero() {
		t.Fatalf("Blocked = %+v, %v; want owner, from Popular", blocks, err)
	}

	for name, err := range map[string]error{
		"profile 1's catalog":    second(db.PublishCatalog(ctx, slot1, c.ID)),
		"profile 2's collection": second(db.PublishCollection(ctx, slot2, coll.ID)),
	} {
		if !errors.Is(err, ErrBlocked) {
			t.Errorf("publish %s while blocked = %v, want ErrBlocked", name, err)
		}
	}
	if _, err := db.PublishCatalog(ctx, other, others.ID); err != nil {
		t.Errorf("another account's publish = %v", err)
	}

	if ok, err := db.Unblock(ctx, "owner"); err != nil || !ok {
		t.Fatalf("Unblock = %v, %v; want it was blocked", ok, err)
	}
	if ok, err := db.Unblock(ctx, "owner"); err != nil || ok {
		t.Errorf("Unblock again = %v, %v; want it wasn't blocked", ok, err)
	}
	if blocks, err := db.Blocked(ctx); err != nil || len(blocks) != 0 {
		t.Errorf("Blocked after unblocking = %+v, %v; want none", blocks, err)
	}
	if _, err := db.PublishCatalog(ctx, slot1, c.ID); err != nil {
		t.Errorf("publish after unblocking = %v", err)
	}
	if _, err := db.PublishCollection(ctx, slot2, coll.ID); err != nil {
		t.Errorf("publish from profile 2 after unblocking = %v", err)
	}
}
