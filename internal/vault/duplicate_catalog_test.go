package vault

import (
	"context"
	"errors"
	"testing"
)

// DuplicateCatalog writes a new listed catalog with the source's recipe and
// " (copy)" after its name, owned, unpublished and subscribed to nothing, and
// leaves the source as it was.
func TestDuplicateCatalog(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	source := publishCatalog(t, db, owner, "Source", `{"sort_by":"popularity.desc"}`)

	dup, err := db.DuplicateCatalog(ctx, owner, source.ID)
	if err != nil {
		t.Fatalf("DuplicateCatalog: %v", err)
	}
	if dup.ID == source.ID || dup.OwnerID != owner || dup.CollectionID != nil {
		t.Errorf("duplicate = %+v, want a new listed catalog of the owner's", dup)
	}
	if dup.Name != "Source (copy)" || dup.Type != source.Type || dup.Provider != source.Provider || dup.Params != source.Params {
		t.Errorf("duplicate = %q %s %s %s, want %q with the source's recipe", dup.Name, dup.Type, dup.Provider, dup.Params, "Source (copy)")
	}
	if dup.Publication != nil || dup.Subscription != nil {
		t.Errorf("duplicate publication = %v, subscription = %v, want neither", dup.Publication, dup.Subscription)
	}
	if after, err := ownCatalog(ctx, db.conn, owner, source.ID); err != nil || after.Name != "Source" || after.Publication == nil {
		t.Errorf("source after duplicating = %+v, %v, want it unchanged and still published", after, err)
	}
}

// Another profile's catalog and a catalog scoped to a collection are both
// ErrCatalogNotFound: Duplicate copies listed catalogs of the caller's own.
func TestDuplicateCatalogNotFound(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, other := newTestProfile(t, db, "owner"), newTestProfile(t, db, "other")
	notMine := createListed(t, db, other, "Not mine")
	collection, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "C", ViewMode: "TABBED_GRID"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	scoped := createScopedCatalog(t, db, owner, collection.ID, listedCatalogForm("Scoped"))

	for name, id := range map[string]Catalog{"another profile's": notMine, "a scoped": scoped} {
		if _, err := db.DuplicateCatalog(ctx, owner, id.ID); !errors.Is(err, ErrCatalogNotFound) {
			t.Errorf("duplicate %s catalog = %v, want ErrCatalogNotFound", name, err)
		}
	}
}

// A Community catalog's Duplicate names the copy the way the library's does.
func TestDuplicatePublicationNamesACatalogCopy(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, duplicator := newTestProfile(t, db, "owner"), newTestProfile(t, db, "duplicator")
	source := publishCatalog(t, db, owner, "Shared", "{}")

	copied, err := db.DuplicatePublication(ctx, duplicator, source.Publication.ID)
	if err != nil {
		t.Fatalf("DuplicatePublication: %v", err)
	}
	if copied.Catalog.Name != "Shared (copy)" {
		t.Errorf("duplicate name = %q, want %q", copied.Catalog.Name, "Shared (copy)")
	}
	subscribed, err := db.Subscribe(ctx, duplicator, source.Publication.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if subscribed.Catalog.Name != "Shared" {
		t.Errorf("subscribed name = %q, want it unchanged", subscribed.Catalog.Name)
	}
}
