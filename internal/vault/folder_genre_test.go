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
		Title: "Source",
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
	taken := takeCollection(t, db, owner, taker, source.ID)

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

// Every folder of a copied collection keeps its own refs, in its own order:
// the refs of a whole tree are loaded in one query grouped by folder, so a
// tree whose folders would interleave if the grouping were wrong comes back
// unmixed on a reload, a Duplicate and a Take alike.
func TestCopiedFolderRefsStayGroupedPerFolder(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	names := []string{"Alpha", "Beta", "Gamma"}
	ids := make([]uuid.UUID, len(names))
	for i, name := range names {
		c, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm(name))
		if err != nil {
			t.Fatalf("create catalog %s: %v", name, err)
		}
		ids[i] = c.ID
	}

	// Each folder names the same three catalogs in a different order, so a
	// lookup that dropped folder_id would hand every folder the same refs.
	wantOrder := [][]int{{0, 1, 2}, {2, 0, 1}, {1, 2, 0}}
	folders := make([]FolderData, len(wantOrder))
	for i, order := range wantOrder {
		refs := make([]FolderCatalogRef, len(order))
		for j, k := range order {
			refs[j] = FolderCatalogRef{CatalogID: &ids[k], Genre: names[k]}
		}
		folders[i] = FolderData{Title: "Folder " + names[i], Catalogs: refs}
	}

	source, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title:   "Interleaved",
		Folders: folders,
	})
	if err != nil {
		t.Fatalf("create source collection: %v", err)
	}

	reloaded, err := db.GetCollectionsByIDs(ctx, []uuid.UUID{source.ID})
	if err != nil || len(reloaded) != 1 {
		t.Fatalf("reload collection: %v (%d rows)", err, len(reloaded))
	}
	dup, err := db.DuplicateCollection(ctx, owner, source.ID)
	if err != nil {
		t.Fatalf("DuplicateCollection: %v", err)
	}
	taken := takeCollection(t, db, owner, taker, source.ID)

	// A copy re-keys refs to its own catalog ids, so the genre — carried
	// verbatim and unique per catalog here — is what identifies each ref.
	for name, c := range map[string]CollectionWithFolders{
		"reload": reloaded[0], "duplicate": dup, "take": taken,
	} {
		if len(c.Folders) != len(wantOrder) {
			t.Fatalf("%s: %d folders, want %d", name, len(c.Folders), len(wantOrder))
		}
		for i, order := range wantOrder {
			want := make([]string, len(order))
			for j, k := range order {
				want[j] = names[k]
			}
			got := make([]string, 0, len(c.Folders[i].Refs))
			for _, ref := range c.Folders[i].Refs {
				got = append(got, ref.Genre)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s: folder %d refs = %v, want %v", name, i, got, want)
			}
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
