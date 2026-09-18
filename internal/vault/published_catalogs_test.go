package vault

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// GetPublishedCatalogs is the union of listed catalogs on the home screen
// plus every catalog referenced by a folder of a collection on the home
// screen, deduped by id with the home row winning ShowInHome.
func TestGetPublishedCatalogs(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")

	onHome, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("On home and in a folder"))
	if err != nil {
		t.Fatalf("create on-home catalog: %v", err)
	}
	folderOnly, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Folder only"))
	if err != nil {
		t.Fatalf("create folder-only catalog: %v", err)
	}
	offTV, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("In a folder of an off-TV collection"))
	if err != nil {
		t.Fatalf("create off-TV catalog: %v", err)
	}

	onTVCollection := newTestCollection(t, db, owner, "On TV")
	// folderOnly is referenced by two folders of the same on-TV collection —
	// branch 2 of the union emits two rows for it (different o2/o3), and
	// plain UNION's row-level DISTINCT does not collapse them since the
	// other scanned columns differ too; only the Go id-keyed dedupe does.
	if _, err := db.UpdateUserCollection(ctx, owner, onTVCollection, CollectionForm{
		Title: "On TV",
		Folders: []FolderData{
			{Title: "Folder 1", Catalogs: CatalogRefs(onHome.ID, folderOnly.ID)},
			{Title: "Folder 2", Catalogs: CatalogRefs(folderOnly.ID)},
		},
	}); err != nil {
		t.Fatalf("saving on-TV collection: %v", err)
	}

	offTVCollection := newTestCollection(t, db, owner, "Off TV")
	if _, err := db.UpdateUserCollection(ctx, owner, offTVCollection, CollectionForm{
		Title:   "Off TV",
		Folders: []FolderData{{Title: "Folder", Catalogs: CatalogRefs(offTV.ID)}},
	}); err != nil {
		t.Fatalf("saving off-TV collection: %v", err)
	}

	// Put onHome on the home screen and onTVCollection on the TV.
	// offTVCollection stays off; folderOnly and offTV are never selected
	// directly.
	if err := db.SaveSelectionsForPush(ctx, owner,
		CatalogSelectionForm{Catalogs: []SelectedCatalogInput{{CatalogID: onHome.ID, ShowInHome: true}}},
		CollectionSelectionForm{CollectionIDs: []uuid.UUID{onTVCollection}},
		nil,
	); err != nil {
		t.Fatalf("SaveSelectionsForPush: %v", err)
	}

	published, err := db.GetPublishedCatalogs(ctx, owner)
	if err != nil {
		t.Fatalf("GetPublishedCatalogs: %v", err)
	}

	byID := make(map[uuid.UUID]SelectedCatalog, len(published))
	for _, sc := range published {
		byID[sc.ID] = sc
	}

	if sc, ok := byID[onHome.ID]; !ok || !sc.ShowInHome {
		t.Fatalf("onHome catalog present=%v, ShowInHome=%v, want present with ShowInHome=true", ok, sc.ShowInHome)
	}
	if sc, ok := byID[folderOnly.ID]; !ok || sc.ShowInHome {
		t.Fatalf("folderOnly catalog present=%v, ShowInHome=%v, want present with ShowInHome=false", ok, sc.ShowInHome)
	}
	if _, ok := byID[offTV.ID]; ok {
		t.Fatalf("offTV catalog appeared in the published set; its collection isn't on the TV")
	}
	if len(published) != 2 {
		t.Fatalf("GetPublishedCatalogs returned %d catalogs, want 2 (onHome, folderOnly), got %+v", len(published), published)
	}

	// The query's ORDER BY rank, o1, o2, o3: the home row comes before any
	// folder-derived row.
	if published[0].ID != onHome.ID || published[1].ID != folderOnly.ID {
		t.Fatalf("GetPublishedCatalogs order = [%s, %s], want [onHome, folderOnly]",
			published[0].ID, published[1].ID)
	}
}
