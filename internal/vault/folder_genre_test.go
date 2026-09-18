package vault

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// A folder ref's genre is stored per reference: it comes back, in order, on
// the save response and on a reload, trimmed, with "" for an unfiltered ref.
// One catalog can appear twice in a folder under two genres.
func TestFolderRefGenreRoundTrips(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	catalog, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Popular"))
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	other, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Other"))
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}

	saved, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title: "Genres",
		Folders: []FolderData{
			{Title: "Mixed", Catalogs: []FolderCatalogRef{
				{CatalogID: &catalog.ID, Genre: "  Western "},
				{CatalogID: &other.ID},
				{CatalogID: &catalog.ID, Genre: "War"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}

	reloaded, err := db.GetCollectionsByIDs(ctx, []uuid.UUID{saved.ID})
	if err != nil || len(reloaded) != 1 {
		t.Fatalf("reload collection: %v (%d rows)", err, len(reloaded))
	}

	want := []FolderRef{
		{CatalogID: catalog.ID, Genre: "Western"},
		{CatalogID: other.ID, Genre: ""},
		{CatalogID: catalog.ID, Genre: "War"},
	}
	for name, c := range map[string]CollectionWithFolders{"save response": saved, "reload": reloaded[0]} {
		if got := c.Folders[0].Refs; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: refs = %+v, want %+v", name, got, want)
		}
		// The referenced catalog is listed once, not once per ref.
		if len(c.Catalogs) != 2 {
			t.Errorf("%s: %d catalogs, want 2", name, len(c.Catalogs))
		}
	}
}

// A folder with no refs serializes them as [], never null.
func TestFolderRefsSerializeEmptyAsArray(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	saved, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title:   "Plain",
		Folders: []FolderData{{Title: "Empty"}},
	})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	reloaded, err := db.GetCollectionsByIDs(ctx, []uuid.UUID{saved.ID})
	if err != nil {
		t.Fatalf("reload collection: %v", err)
	}

	for name, c := range map[string]CollectionWithFolders{"save response": saved, "reload": reloaded[0]} {
		raw, err := json.Marshal(c.Folders[0])
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !strings.Contains(string(raw), `"refs":[]`) {
			t.Errorf("%s: folder JSON = %s, want refs []", name, raw)
		}
	}
}

// Duplicate and Take both carry every ref's genre onto the copy, including
// two refs to one catalog, re-keyed to the copy's catalog id where the
// catalog itself was copied.
func TestFolderRefGenreSurvivesDuplicateAndTake(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	catalog, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Popular"))
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	source, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title:    "Source",
		IsPublic: true,
		Folders: []FolderData{{Title: "Mixed", Catalogs: []FolderCatalogRef{
			{CatalogID: &catalog.ID, Genre: "Western"},
			{CatalogID: &catalog.ID, Genre: "War"},
		}}},
	})
	if err != nil {
		t.Fatalf("create source collection: %v", err)
	}

	dup, err := db.DuplicateCollection(ctx, owner, source.ID)
	if err != nil {
		t.Fatalf("DuplicateCollection: %v", err)
	}
	taken, err := db.TakeCollection(ctx, taker, source.ID)
	if err != nil {
		t.Fatalf("TakeCollection: %v", err)
	}

	for name, c := range map[string]CollectionWithFolders{"duplicate": dup, "take": taken} {
		refs := c.Folders[0].Refs
		if len(refs) != 2 {
			t.Fatalf("%s: folder has %d refs, want 2", name, len(refs))
		}
		if refs[0].CatalogID != refs[1].CatalogID {
			t.Errorf("%s: refs point at %s and %s, want one catalog twice", name, refs[0].CatalogID, refs[1].CatalogID)
		}
		if refs[0].Genre != "Western" || refs[1].Genre != "War" {
			t.Errorf("%s: genres = %q, %q, want Western, War", name, refs[0].Genre, refs[1].Genre)
		}
	}
}

// The same catalog under the same genre twice in one folder is rejected as
// invalid input rather than reaching the primary key; after trimming, "War"
// and " War" are the same genre.
func TestFolderRefSameCatalogSameGenreIsInvalid(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	catalog, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Popular"))
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	for _, genres := range [][2]string{{"", ""}, {"War", " War"}} {
		_, err = db.CreateUserCollection(ctx, owner, CollectionForm{
			Title: "Repeat",
			Folders: []FolderData{{Title: "F", Catalogs: []FolderCatalogRef{
				{CatalogID: &catalog.ID, Genre: genres[0]},
				{CatalogID: &catalog.ID, Genre: genres[1]},
			}}},
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("genres %q: got %v, want ErrInvalidInput", genres, err)
		}
	}
}

func TestFolderRefGenreTooLongIsInvalid(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	catalog, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Popular"))
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	_, err = db.CreateUserCollection(ctx, owner, CollectionForm{
		Title: "Too long",
		Folders: []FolderData{{Title: "F", Catalogs: []FolderCatalogRef{
			{CatalogID: &catalog.ID, Genre: strings.Repeat("x", maxGenreLen+1)},
		}}},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("create with an overlong genre: got %v, want ErrInvalidInput", err)
	}
}

// A New entry needs a Key; entries sharing one must carry the same spec, and
// may not repeat a genre within a folder, since they become one catalog.
func TestFolderNewRefKeyIsValidated(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	spec := NewScopedCatalog{Key: "draft:a", Type: "movie", Name: "Staged", Provider: "tmdb", Params: "{}"}
	renamed := spec
	renamed.Name = "Renamed"
	unkeyed := spec
	unkeyed.Key = ""

	cases := map[string][]FolderCatalogRef{
		"missing key":       {{New: &unkeyed}},
		"shared key, spec":  {{New: &spec, Genre: "Action"}, {New: &renamed, Genre: "Comedy"}},
		"shared key, genre": {{New: &spec, Genre: "War"}, {New: &spec, Genre: " War"}},
	}
	for name, refs := range cases {
		_, err := db.CreateUserCollection(ctx, owner, CollectionForm{
			Title:   "Staged",
			Folders: []FolderData{{Title: "F", Catalogs: refs}},
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: got %v, want ErrInvalidInput", name, err)
		}
	}
}
