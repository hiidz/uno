package vault

import (
	"context"
	"testing"
)

// The Nuvio appearance fields (focus glow on the collection; focus GIF and
// Modern Home hero backdrop and title logo on a folder) survive create, an in-place update of
// an existing folder, and DuplicateCollection's tree copy.
func TestCollectionAppearanceFieldsRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	listed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatalf("create listed catalog: %v", err)
	}

	folder := FolderData{
		Title: "Folder",
		FolderArt: FolderArt{TileShape: "POSTER",
			FocusGIFURL:     "https://example.com/focus.gif",
			FocusGIFEnabled: true,
			HeroBackdropURL: "https://example.com/backdrop.jpg",
			TitleLogoURL:    "https://example.com/logo.png",
		},
		Catalogs: CatalogRefs(listed.ID),
	}
	created, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title: "C", ViewMode: "TABBED_GRID",
		FocusGlowEnabled: true,
		Folders:          []FolderData{folder},
	})
	if err != nil {
		t.Fatalf("CreateUserCollection: %v", err)
	}
	if !created.FocusGlowEnabled {
		t.Errorf("created focus_glow_enabled = false, want true")
	}
	assertFolderAppearance(t, "created", created.Folders[0].Folder, folder)

	folder.ID = &created.Folders[0].ID
	folder.FocusGIFEnabled = false
	folder.TitleLogoURL = "https://example.com/logo2.png"
	updated, err := db.UpdateUserCollection(ctx, owner, created.ID, collectionRevision(t, db, created.ID), CollectionForm{
		Title: "C", ViewMode: "TABBED_GRID",
		FocusGlowEnabled: false,
		Folders:          []FolderData{folder},
	})
	if err != nil {
		t.Fatalf("UpdateUserCollection: %v", err)
	}

	reloaded, err := db.GetUserCollections(ctx, owner)
	if err != nil {
		t.Fatalf("GetUserCollections: %v", err)
	}
	if len(reloaded) != 1 || len(reloaded[0].Folders) != 1 {
		t.Fatalf("reloaded collections = %+v, want one collection with one folder", reloaded)
	}
	if reloaded[0].FocusGlowEnabled {
		t.Errorf("reloaded focus_glow_enabled = true, want false")
	}
	assertFolderAppearance(t, "reloaded", reloaded[0].Folders[0].Folder, folder)

	dup, err := db.DuplicateCollection(ctx, owner, updated.ID)
	if err != nil {
		t.Fatalf("DuplicateCollection: %v", err)
	}
	if dup.FocusGlowEnabled {
		t.Errorf("duplicate focus_glow_enabled = true, want false")
	}
	assertFolderAppearance(t, "duplicate", dup.Folders[0].Folder, folder)
}

func assertFolderAppearance(t *testing.T, label string, got Folder, want FolderData) {
	t.Helper()
	if got.FocusGIFURL != want.FocusGIFURL || got.FocusGIFEnabled != want.FocusGIFEnabled ||
		got.HeroBackdropURL != want.HeroBackdropURL || got.TitleLogoURL != want.TitleLogoURL {
		t.Errorf("%s folder appearance = {gif %q %v, backdrop %q, logo %q}, want {gif %q %v, backdrop %q, logo %q}",
			label, got.FocusGIFURL, got.FocusGIFEnabled, got.HeroBackdropURL, got.TitleLogoURL,
			want.FocusGIFURL, want.FocusGIFEnabled, want.HeroBackdropURL, want.TitleLogoURL)
	}
}

// A hero video that arrives with a folder is never stored, on create or on an
// in-place update.
func TestHeroVideoIsNeverStored(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	listed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatalf("create listed catalog: %v", err)
	}
	folder := FolderData{
		Title:     "Folder",
		FolderArt: FolderArt{TileShape: "POSTER", HeroVideoURL: "https://example.com/hero.mp4"},
		Catalogs:  CatalogRefs(listed.ID),
	}
	form := CollectionForm{Title: "C", ViewMode: "TABBED_GRID", Folders: []FolderData{folder}}
	created, err := db.CreateUserCollection(ctx, owner, form)
	if err != nil {
		t.Fatalf("CreateUserCollection: %v", err)
	}
	if got := created.Folders[0].HeroVideoURL; got != "" {
		t.Errorf("created hero video = %q, want none", got)
	}

	form.Folders[0].ID = &created.Folders[0].ID
	if _, err := db.UpdateUserCollection(ctx, owner, created.ID, collectionRevision(t, db, created.ID), form); err != nil {
		t.Fatalf("UpdateUserCollection: %v", err)
	}
	reloaded, err := db.GetUserCollections(ctx, owner)
	if err != nil {
		t.Fatalf("GetUserCollections: %v", err)
	}
	if got := reloaded[0].Folders[0].HeroVideoURL; got != "" {
		t.Errorf("updated hero video = %q, want none", got)
	}
}
