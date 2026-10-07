// Snapshots: the frozen form a publication holds of its source. A snapshot
// is the bundle form with every catalog's params inline and every catalog
// and folder under a stable key, so a subscriber's copy is paired with the
// snapshot row by row across republishes. Its exact bytes are stored, and
// their hash is the publication's content hash.

package vault

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
)

// The snapshot format. Changing what a snapshot holds, or how it is encoded,
// changes every content hash, and every subscriber would see an update that
// isn't one, so it is a schema change (schemaVersion).
const (
	SnapshotFormat  = "uno-publication"
	SnapshotVersion = 1
)

// Snapshot is what a publication holds: every catalog, with its params
// inline, under its stable key, and for a collection its own fields and
// folders, each folder under its stable key, with refs naming catalogs by
// key. A catalog publication holds one catalog and no collection.
type Snapshot struct {
	Format     string              `json:"format"`
	Version    int                 `json:"version"`
	Catalogs   []BundleCatalog     `json:"catalogs"`
	Collection *SnapshotCollection `json:"collection,omitempty"`
}

// SnapshotCollection is a collection publication's own fields and folders.
type SnapshotCollection struct {
	Title            string           `json:"title"`
	ViewMode         string           `json:"view_mode"`
	ShowAllTab       bool             `json:"show_all_tab"`
	BackdropImageURL string           `json:"backdrop_image_url"`
	FocusGlowEnabled bool             `json:"focus_glow_enabled"`
	Folders          []SnapshotFolder `json:"folders"`
}

// SnapshotFolder is one folder of a SnapshotCollection, under its key.
type SnapshotFolder struct {
	Key string `json:"key"`
	BundleFolder
}

// stableKey is the key a snapshot of publication publicationID gives the
// catalog or folder sourceID: the first 16 hex digits of the sha256 of the
// two ids, so the same row has the same key in every snapshot the
// publication holds.
func stableKey(publicationID, sourceID uuid.UUID) string {
	return sha256Hex([]byte(publicationID.String() + sourceID.String()))[:16]
}

// catalogSnapshot is the snapshot of catalog c published as publicationID.
func catalogSnapshot(publicationID uuid.UUID, c Catalog) Snapshot {
	return catalogSnapshotKeyed(stableKey(publicationID, c.ID), c)
}

// catalogSnapshotKeyed is the snapshot of catalog c under key.
func catalogSnapshotKeyed(key string, c Catalog) Snapshot {
	return Snapshot{Format: SnapshotFormat, Version: SnapshotVersion, Catalogs: []BundleCatalog{bundleCatalogFrom(key, c)}}
}

// collectionSnapshot is the snapshot of tree published as publicationID:
// every catalog its folders reference, in the order they are first
// referenced, and its folders, each under the stable key of its row.
func collectionSnapshot(publicationID uuid.UUID, tree CollectionWithFolders) Snapshot {
	return keyedSnapshot(tree,
		func(c Catalog) string { return stableKey(publicationID, c.ID) },
		func(f Folder) string { return stableKey(publicationID, f.ID) })
}

// keyedSnapshot is the snapshot of tree with each catalog under catalogKey
// and each folder under folderKey.
func keyedSnapshot(tree CollectionWithFolders, catalogKey func(Catalog) string, folderKey func(Folder) string) Snapshot {
	bc := extractBundle(nil, []CollectionWithFolders{tree}, true).Collections[0]
	byID := make(map[uuid.UUID]Catalog, len(tree.Catalogs))
	for _, c := range tree.Catalogs {
		byID[c.ID] = c
	}
	keys := make(map[string]string, len(bc.Catalogs))
	catalogs := make([]BundleCatalog, len(bc.Catalogs))
	for i, c := range bc.Catalogs {
		keys[c.Key] = catalogKey(byID[*c.SourceID])
		c.Key = keys[c.Key]
		catalogs[i] = c
	}
	folders := make([]SnapshotFolder, len(bc.Folders))
	for i, f := range bc.Folders {
		folders[i] = SnapshotFolder{Key: folderKey(tree.Folders[i].Folder), BundleFolder: rekeyRefs(f, keys)}
	}
	return Snapshot{
		Format: SnapshotFormat, Version: SnapshotVersion, Catalogs: catalogs,
		Collection: &SnapshotCollection{
			Title: bc.Title, ViewMode: bc.ViewMode, ShowAllTab: bc.ShowAllTab,
			BackdropImageURL: bc.BackdropImageURL, FocusGlowEnabled: bc.FocusGlowEnabled, Folders: folders,
		},
	}
}

// rekeyRefs is f with every ref's catalog key replaced by the one keys maps
// it to.
func rekeyRefs(f BundleFolder, keys map[string]string) BundleFolder {
	refs := make([]BundleRef, len(f.Refs))
	for i, ref := range f.Refs {
		refs[i] = BundleRef{Catalog: keys[ref.Catalog], Genre: ref.Genre}
	}
	f.Refs = refs
	return f
}

// encode is s's stored bytes and their hash, the content hash. Everything
// a snapshot holds encodes but a catalog's params, which a stored row can
// hold as something other than JSON, so a failure is ErrInvalidInput.
func (s Snapshot) encode() (string, string, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return "", "", fmt.Errorf("%w: a catalog's params are not JSON: %w", ErrInvalidInput, err)
	}
	return string(b), sha256Hex(b), nil
}

// contentHash is the hash of s's stored bytes, or "" when s doesn't encode.
func (s Snapshot) contentHash() string {
	_, hash, _ := s.encode()
	return hash
}

// decodeSnapshot reads a stored snapshot.
func decodeSnapshot(raw string) (Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return Snapshot{}, fmt.Errorf("decoding snapshot: %w", err)
	}
	return s, nil
}

// bundleCollection is s's collection in bundle form: its catalogs as the
// collection's own, and its folders without their keys. Only a collection
// snapshot has one.
func (s Snapshot) bundleCollection() BundleCollection {
	c := s.Collection
	folders := make([]BundleFolder, len(c.Folders))
	for i, f := range c.Folders {
		folders[i] = f.BundleFolder
	}
	return BundleCollection{
		Title: c.Title, ViewMode: c.ViewMode, ShowAllTab: c.ShowAllTab,
		BackdropImageURL: c.BackdropImageURL, FocusGlowEnabled: c.FocusGlowEnabled,
		Catalogs: s.Catalogs, Folders: folders,
	}
}

// catalogForm is the form of s's only catalog, a catalog snapshot's.
func (s Snapshot) catalogForm() CatalogForm {
	c := s.Catalogs[0]
	return CatalogForm{Type: c.Type, Name: c.Name, Provider: c.Provider, Params: string(c.Params)}
}

// collectionForm is the form that writes s's collection as a new
// collection: every catalog a New entry, carrying its key as SubKey when
// keyed, and every folder's SubKey its key when keyed.
func (s Snapshot) collectionForm(keyed bool) CollectionForm {
	form := collectionFormFromBundle(s.bundleCollection(), nil, keyed)
	if keyed {
		for i := range form.Folders {
			form.Folders[i].SubKey = s.Collection.Folders[i].Key
		}
	}
	return form
}

// forCopy is s as a copy of it is written: a subscribe's is s as it is, and a
// duplicate's has its catalog's name or its collection's title given a
// " (copy)" suffix (copyName). s is left as it was.
func (s Snapshot) forCopy(subscribe bool) Snapshot {
	if subscribe {
		return s
	}
	if s.Collection == nil {
		s.Catalogs = slices.Clone(s.Catalogs)
		s.Catalogs[0].Name = copyName(s.Catalogs[0].Name)
		return s
	}
	c := *s.Collection
	c.Title = copyName(c.Title)
	s.Collection = &c
	return s
}

// validate runs the form validators a copy of s is written through: a
// catalog save's for a catalog snapshot, a collection save's for a
// collection snapshot. A subscribe or a duplicate runs only this over s, since
// the recipes were checked against TMDB when s was published; a publish runs
// it before that check, and an Update runs the same validators over the form
// it writes.
func (s Snapshot) validate() error {
	if s.Collection == nil {
		return s.catalogForm().Validate()
	}
	return s.collectionForm(false).Validate()
}
