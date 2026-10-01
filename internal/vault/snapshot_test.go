package vault

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// fixedID is a UUID whose last byte is n, for trees with ids a test can pin.
func fixedID(n byte) uuid.UUID { return uuid.UUID{15: n} }

// fixedTree is a collection tree with fixed ids: a listed catalog referenced
// twice and a scoped one, in two folders, every field set.
func fixedTree() CollectionWithFolders {
	collectionID := fixedID(1)
	listed := Catalog{ID: fixedID(2), Type: "movie", Name: "Popular", Provider: "tmdb", Params: `{"sort_by":"popularity.desc"}`}
	scoped := Catalog{ID: fixedID(3), Type: "series", Name: "Ghosts & Ghouls", Provider: "tmdb", Params: `{"with_keywords":"162846"}`, CollectionID: &collectionID}
	return CollectionWithFolders{
		Collection: Collection{ID: collectionID, Title: "Halloween <3", ViewMode: "ROWS", ShowAllTab: true,
			BackdropImageURL: "https://example.com/b.jpg", FocusGlowEnabled: true},
		Folders: []FolderWithCatalogs{
			{Folder: Folder{ID: fixedID(4), Title: "Classics", TileShape: "POSTER", CoverEmoji: "🎃", FocusGIFEnabled: true,
				CoverImageURL: "https://example.com/c.png", FocusGIFURL: "https://example.com/f.gif", HeroBackdropURL: "https://example.com/h.jpg",
				HeroVideoURL: "https://example.com/h.mp4", TitleLogoURL: "https://example.com/l.png"},
				Refs: []FolderRef{{listed.ID, ""}, {scoped.ID, "Horror"}}},
			{Folder: Folder{ID: fixedID(5), Title: "Modern", TileShape: "SQUARE", HideTitle: true}, Refs: []FolderRef{{listed.ID, "War"}}},
		},
		Catalogs: []Catalog{listed, scoped},
	}
}

// A snapshot's bytes are pinned, because every stored content_hash and every
// subscription's taken_hash was computed from them. Anything that moves them
// — a field added to, renamed in or reordered in the snapshot or bundle
// form, another stable key — makes every subscriber see an update that
// changes nothing. A new value here is a schema change (schemaVersion).
func TestSnapshotIsPinned(t *testing.T) {
	publicationID := fixedID(9)
	tree := fixedTree()
	for _, tc := range []struct {
		name string
		s    Snapshot
		want string
	}{
		{"collection", collectionSnapshot(publicationID, tree), "0d98d921ae74a3ddb878608b6a461857a59d27394885f6490dac524eb905929c"},
		{"catalog", catalogSnapshot(publicationID, tree.Catalogs[0]), "dfe7192dbd7a5af0efa692f15ad308a3c34f4e1dab80b5469df39d04f9f1f544"},
	} {
		raw, hash, err := tc.s.encode()
		if err != nil {
			t.Fatalf("%s: encode: %v", tc.name, err)
		}
		if hash != tc.want {
			t.Errorf("%s snapshot hash = %s, pinned %s; every stored content_hash would read as changed\n%s", tc.name, hash, tc.want, raw)
		}
	}
}

// A collection's snapshot holds every catalog its folders reference once,
// in the order they are first referenced, and every folder, each under the
// stable key of its row; refs name catalogs by key; no row id is in it.
func TestCollectionSnapshotShape(t *testing.T) {
	publicationID := fixedID(9)
	tree := fixedTree()
	s := collectionSnapshot(publicationID, tree)
	key := func(id uuid.UUID) string { return stableKey(publicationID, id) }

	assertStrings(t, "catalogs", bundleCatalogKeys(s.Catalogs), []string{key(fixedID(2)) + "=Popular", key(fixedID(3)) + "=Ghosts & Ghouls"})
	if s.Collection.Folders[0].Key != key(fixedID(4)) || s.Collection.Folders[1].Key != key(fixedID(5)) {
		t.Errorf("folder keys = %s, %s", s.Collection.Folders[0].Key, s.Collection.Folders[1].Key)
	}
	assertStrings(t, "first folder's refs", bundleFolderRefs(s.Collection.Folders[0].BundleFolder), []string{key(fixedID(2)) + "/", key(fixedID(3)) + "/Horror"})
	if s.folderCount() != 2 {
		t.Errorf("folders %d; want 2", s.folderCount())
	}

	raw, _, err := s.encode()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{fixedID(1), fixedID(2), fixedID(3), fixedID(4)} {
		if strings.Contains(raw, id.String()) {
			t.Errorf("snapshot holds row id %s: %s", id, raw)
		}
	}
	decoded, err := decodeSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if again, _, _ := decoded.encode(); again != raw {
		t.Errorf("decoded snapshot re-encodes as %s, want %s", again, raw)
	}
}

// An empty collection's snapshot has no folders and an empty catalog list,
// and a catalog's has no folders.
func TestSnapshotFolderCount(t *testing.T) {
	c := fixedTree().Catalogs[1]
	empty := CollectionWithFolders{Collection: Collection{ID: fixedID(1), Title: "Empty"}}
	if got := collectionSnapshot(fixedID(9), empty); got.folderCount() != 0 || got.Catalogs == nil {
		t.Errorf("empty collection snapshot = %+v, want no folders and an empty catalog list", got)
	}
	if got := catalogSnapshot(fixedID(9), c).folderCount(); got != 0 {
		t.Errorf("catalog snapshot folders = %d, want 0", got)
	}
}

// The keyed form of a collection snapshot writes each catalog and folder
// with its key as SubKey; the unkeyed one, a fork's, with none.
func TestSnapshotCollectionForm(t *testing.T) {
	s := collectionSnapshot(fixedID(9), fixedTree())
	keyed := s.collectionForm(true)
	if keyed.Folders[1].SubKey != s.Collection.Folders[1].Key || keyed.Folders[0].Catalogs[1].New.SubKey != s.Catalogs[1].Key {
		t.Errorf("keyed form = %+v, want every folder and catalog under its key", keyed)
	}
	plain := s.collectionForm(false)
	if plain.Folders[1].SubKey != "" || plain.Folders[0].Catalogs[1].New.SubKey != "" {
		t.Errorf("unkeyed form = %+v, want no sub_keys", plain)
	}
	if plain.Folders[0].Catalogs[0].Genre != "" || plain.Folders[0].Catalogs[1].Genre != "Horror" {
		t.Errorf("genres = %+v", plain.Folders[0].Catalogs)
	}
	if err := s.validate(); err != nil {
		t.Errorf("validate = %v, want nil", err)
	}
}

// A snapshot today's rules refuse fails validate, a catalog snapshot by the
// catalog rules and a collection one by the collection rules; params that
// are not JSON fail to encode as ErrInvalidInput; a stored snapshot that
// isn't JSON fails to decode.
func TestSnapshotRefusals(t *testing.T) {
	c := fixedTree().Catalogs[0]
	c.Name = " "
	if err := catalogSnapshot(fixedID(9), c).validate(); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("blank-named catalog snapshot validate = %v, want ErrInvalidInput", err)
	}
	tree := fixedTree()
	tree.ViewMode = "CAROUSEL"
	if err := collectionSnapshot(fixedID(9), tree).validate(); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("collection snapshot with an unknown view mode validate = %v, want ErrInvalidInput", err)
	}
	c.Params = "not json"
	if _, _, err := catalogSnapshot(fixedID(9), c).encode(); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("encode with params that aren't JSON = %v, want ErrInvalidInput", err)
	}
	if got := catalogSnapshot(fixedID(9), c).contentHash(); got != "" {
		t.Errorf("content hash of a snapshot that doesn't encode = %q, want empty", got)
	}
	if _, err := decodeSnapshot("not json"); err == nil {
		t.Error("decodeSnapshot of text that isn't JSON = nil, want an error")
	}
}
