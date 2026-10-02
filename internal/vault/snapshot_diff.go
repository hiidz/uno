// The comparison of two snapshots: what a row would gain, lose and change
// going from one to the other, as structured items the SPA puts into words.
// It is the one comparison behind every "what changed" list — a row added
// from Community against its publisher's latest version, and an own row
// against what it last published — and it matches rows by snapshot key, so
// it reads the content, never the order the rows were written in: an edit
// and its undo are no change.

package vault

import (
	"encoding/json"
	"slices"
)

// The ops and kinds of a SnapshotChange.
const (
	changeRemoved = "removed"
	changeAdded   = "added"
	changeChanged = "changed"

	changeCollection = "collection"
	changeFolder     = "folder"
	changeCatalog    = "catalog"
)

// What a changed row changed. A catalog's name or recipe; a collection's
// name or settings, or the order or art of its folders; a folder's name, art
// or the order of its catalogs.
const (
	aspectName         = "name"
	aspectRecipe       = "recipe"
	aspectSettings     = "settings"
	aspectArt          = "art"
	aspectOrder        = "order"
	aspectCatalogOrder = "catalog_order"
)

// SnapshotChange is one difference between two snapshots. Removed items
// come first, then added, then changed, each in folder order.
//
//   - A folder is removed or added whole, and so is each catalog it holds
//     that the other snapshot has nowhere: Folder names the folder the
//     catalog sits in, and Genre the genre it is narrowed to there. A
//     catalog the other snapshot keeps in another folder is removed from or
//     added to a folder alone.
//   - A catalog whose name or recipe changed is one item, however many
//     folders use it, carrying the catalog as it is now (Catalog) and as it
//     was (WasCatalog) when the recipe changed; Was is its earlier name when
//     that changed too.
//   - A collection's or a folder's name changed carries its Was. The
//     folders' order and a collection's settings are one item each, and a
//     folder's art or the order of its catalogs one item per folder, named.
type SnapshotChange struct {
	Op         string         `json:"op"`
	Kind       string         `json:"kind"`
	Aspect     string         `json:"aspect,omitempty"`
	Name       string         `json:"name,omitempty"`
	Was        string         `json:"was,omitempty"`
	Folder     string         `json:"folder,omitempty"`
	Genre      string         `json:"genre,omitempty"`
	Catalog    *BundleCatalog `json:"catalog,omitempty"`
	WasCatalog *BundleCatalog `json:"was_catalog,omitempty"`
}

// diffSnapshots is what going from one snapshot to the other changes, empty
// exactly when the two hold the same content.
func diffSnapshots(from, to Snapshot) []SnapshotChange {
	changes := []SnapshotChange{}
	changes = append(changes, onlyIn(from, to, changeRemoved)...)
	changes = append(changes, onlyIn(to, from, changeAdded)...)
	changes = append(changes, edits(from, to)...)
	return changes
}

// snapshotFolders is s's folders in order; a catalog snapshot has none.
func snapshotFolders(s Snapshot) []SnapshotFolder {
	if s.Collection == nil {
		return nil
	}
	return s.Collection.Folders
}

// snapshotFoldersByKey is s's folders by key.
func snapshotFoldersByKey(s Snapshot) map[string]SnapshotFolder {
	byKey := map[string]SnapshotFolder{}
	for _, f := range snapshotFolders(s) {
		byKey[f.Key] = f
	}
	return byKey
}

// snapshotCatalogsByKey is s's catalogs by key.
func snapshotCatalogsByKey(s Snapshot) map[string]BundleCatalog {
	byKey := map[string]BundleCatalog{}
	for _, c := range s.Catalogs {
		byKey[c.Key] = c
	}
	return byKey
}

// holding is what snapshot a holds that snapshot b doesn't, as op. It names
// the catalogs of a, knows b's folders and catalogs, and remembers which
// catalogs b lacks it has already reported, so one in two folders reports once.
type holding struct {
	op           string
	catalogs     map[string]BundleCatalog
	otherFolders map[string]SnapshotFolder
	otherHas     map[string]BundleCatalog
	reported     map[string]bool
}

// onlyIn is what snapshot a holds that snapshot b doesn't, each as op: a
// folder b lacks, then the catalogs it holds that b has nowhere; in a folder b
// keeps, the refs b's version of it lacks; and last any catalog b lacks that
// no folder of a holds.
func onlyIn(a, b Snapshot, op string) []SnapshotChange {
	h := holding{
		op: op, catalogs: snapshotCatalogsByKey(a), otherFolders: snapshotFoldersByKey(b),
		otherHas: snapshotCatalogsByKey(b), reported: map[string]bool{},
	}
	var out []SnapshotChange
	for _, f := range snapshotFolders(a) {
		out = append(out, h.folder(f)...)
	}
	return append(out, h.strays(a)...)
}

// folder is folder f as h's op: the folder and the catalogs only it holds
// when b lacks it, else the refs b's version lacks.
func (h holding) folder(f SnapshotFolder) []SnapshotChange {
	if other, kept := h.otherFolders[f.Key]; kept {
		return h.refs(f, refsBeyond(f.Refs, other.Refs))
	}
	whole := SnapshotChange{Op: h.op, Kind: changeFolder, Name: f.Title}
	return append([]SnapshotChange{whole}, h.refs(f, h.gone(f.Refs))...)
}

// gone is refs with every one to a catalog b has nowhere kept: a folder b
// lacks takes with it only the catalogs b lacks too.
func (h holding) gone(refs []BundleRef) []BundleRef {
	var gone []BundleRef
	for _, ref := range refs {
		if _, has := h.otherHas[ref.Catalog]; !has {
			gone = append(gone, ref)
		}
	}
	return gone
}

// refs is refs, all of folder f's, as catalogs added to or removed from it. A
// catalog b lacks entirely is reported once, in the first folder that holds it.
func (h holding) refs(f SnapshotFolder, refs []BundleRef) []SnapshotChange {
	var out []SnapshotChange
	for _, ref := range refs {
		if !h.reportable(ref.Catalog) {
			continue
		}
		out = append(out, SnapshotChange{Op: h.op, Kind: changeCatalog, Name: h.catalogs[ref.Catalog].Name, Folder: f.Title, Genre: ref.Genre})
	}
	return out
}

// reportable is whether the catalog under key is one to report now: a catalog
// b has is, always, and one it lacks is until it has been reported.
func (h holding) reportable(key string) bool {
	if _, has := h.otherHas[key]; has {
		return true
	}
	first := !h.reported[key]
	h.reported[key] = true
	return first
}

// strays is the catalogs of a that b has nowhere and no folder of a reported.
func (h holding) strays(a Snapshot) []SnapshotChange {
	var out []SnapshotChange
	for _, c := range a.Catalogs {
		if _, has := h.otherHas[c.Key]; !has && !h.reported[c.Key] {
			out = append(out, SnapshotChange{Op: h.op, Kind: changeCatalog, Name: c.Name})
		}
	}
	return out
}

// refsBeyond is the refs of have that other doesn't hold as many of: a
// folder may reference one catalog twice, narrowed to different genres.
func refsBeyond(have, other []BundleRef) []BundleRef {
	counts := map[BundleRef]int{}
	for _, ref := range other {
		counts[ref]++
	}
	var beyond []BundleRef
	for _, ref := range have {
		if counts[ref] > 0 {
			counts[ref]--
			continue
		}
		beyond = append(beyond, ref)
	}
	return beyond
}

// edits is what changed in the rows both snapshots hold: the collection, its
// folders, its catalogs, then the order of its folders.
func edits(from, to Snapshot) []SnapshotChange {
	var out []SnapshotChange
	if from.Collection != nil && to.Collection != nil {
		out = append(out, collectionEdits(*from.Collection, *to.Collection)...)
	}
	out = append(out, folderEdits(from, to)...)
	out = append(out, catalogEdits(from, to)...)
	if !slices.Equal(keptFolderKeys(from, to), keptFolderKeys(to, from)) {
		out = append(out, SnapshotChange{Op: changeChanged, Kind: changeCollection, Aspect: aspectOrder})
	}
	return out
}

// collectionEdits is a collection's name and settings changing.
func collectionEdits(from, to SnapshotCollection) []SnapshotChange {
	var out []SnapshotChange
	if from.Title != to.Title {
		out = append(out, SnapshotChange{Op: changeChanged, Kind: changeCollection, Aspect: aspectName, Name: to.Title, Was: from.Title})
	}
	if collectionSettings(from) != collectionSettings(to) {
		out = append(out, SnapshotChange{Op: changeChanged, Kind: changeCollection, Aspect: aspectSettings})
	}
	return out
}

// settings are a collection's own fields besides its name and folders.
type settings struct {
	viewMode         string
	showAllTab       bool
	backdropImageURL string
	focusGlowEnabled bool
}

func collectionSettings(c SnapshotCollection) settings {
	return settings{c.ViewMode, c.ShowAllTab, c.BackdropImageURL, c.FocusGlowEnabled}
}

// folderEdits is each folder both snapshots hold, in to's order, changing its
// name, its art or the order of its catalogs.
func folderEdits(from, to Snapshot) []SnapshotChange {
	var out []SnapshotChange
	fromFolders := snapshotFoldersByKey(from)
	for _, f := range snapshotFolders(to) {
		old, ok := fromFolders[f.Key]
		if !ok {
			continue
		}
		out = append(out, folderEdit(old, f)...)
	}
	return out
}

// folderEdit is what changed in folder f since old, the same folder.
func folderEdit(old, f SnapshotFolder) []SnapshotChange {
	var out []SnapshotChange
	if old.Title != f.Title {
		out = append(out, SnapshotChange{Op: changeChanged, Kind: changeFolder, Aspect: aspectName, Name: f.Title, Was: old.Title})
	}
	if folderArt(old.BundleFolder) != folderArt(f.BundleFolder) {
		out = append(out, SnapshotChange{Op: changeChanged, Kind: changeFolder, Aspect: aspectArt, Name: f.Title})
	}
	if reordered(old.Refs, f.Refs) {
		out = append(out, SnapshotChange{Op: changeChanged, Kind: changeFolder, Aspect: aspectCatalogOrder, Name: f.Title})
	}
	return out
}

// reordered is whether a and b hold the same refs in another order.
func reordered(a, b []BundleRef) bool {
	return len(refsBeyond(a, b)) == 0 && len(refsBeyond(b, a)) == 0 && !slices.Equal(a, b)
}

// art is how a folder looks: everything it holds but its title and refs.
type art struct {
	tileShape       string
	hideTitle       bool
	coverEmoji      string
	coverImageURL   string
	focusGIFURL     string
	focusGIFEnabled bool
	heroBackdropURL string
	heroVideoURL    string
	titleLogoURL    string
}

func folderArt(f BundleFolder) art {
	return art{f.TileShape, f.HideTitle, f.CoverEmoji, f.CoverImageURL, f.FocusGIFURL, f.FocusGIFEnabled,
		f.HeroBackdropURL, f.HeroVideoURL, f.TitleLogoURL}
}

// catalogEdits is each catalog both snapshots hold, in to's order, whose name
// or recipe changed: one item that carries the catalog when its recipe did.
func catalogEdits(from, to Snapshot) []SnapshotChange {
	var out []SnapshotChange
	fromCatalogs := snapshotCatalogsByKey(from)
	for _, c := range to.Catalogs {
		if change, ok := catalogEdit(fromCatalogs[c.Key], c); ok {
			out = append(out, change)
		}
	}
	return out
}

// catalogEdit is how catalog c differs from old, the same catalog, if it
// does; an old with no key is a catalog from has not got.
func catalogEdit(old, c BundleCatalog) (SnapshotChange, bool) {
	recipe := recipeBytes(old) != recipeBytes(c)
	if old.Key == "" || (!recipe && old.Name == c.Name) {
		return SnapshotChange{}, false
	}
	change := SnapshotChange{Op: changeChanged, Kind: changeCatalog, Aspect: aspectName, Name: c.Name, Was: earlierName(old, c)}
	if recipe {
		change.Aspect, change.Catalog, change.WasCatalog = aspectRecipe, &c, &old
	}
	return change, true
}

// earlierName is old's name when c, the same catalog, was renamed.
func earlierName(old, c BundleCatalog) string {
	if old.Name == c.Name {
		return ""
	}
	return old.Name
}

// recipeBytes is what c's recipe is in a snapshot's stored bytes: its type,
// provider and params as they encode.
func recipeBytes(c BundleCatalog) string {
	params, _ := json.Marshal(c.Params)
	return c.Type + "\n" + c.Provider + "\n" + string(params)
}

// keptFolderKeys is the keys of a's folders that b has too, in a's order.
func keptFolderKeys(a, b Snapshot) []string {
	bFolders := snapshotFoldersByKey(b)
	var keys []string
	for _, f := range snapshotFolders(a) {
		if _, ok := bFolders[f.Key]; ok {
			keys = append(keys, f.Key)
		}
	}
	return keys
}
