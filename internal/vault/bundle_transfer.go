// Export and import: a selection of this profile's own catalogs and
// collections written out as a Bundle, and a Bundle written back in as new
// rows through the same catalog INSERT and collection create core every
// other write uses.

package vault

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/jsonwire"
)

// ExportBundle is the Bundle of the listed catalogs catalogIDs and the
// collections collectionIDs, all of which profileID must own; a scoped
// catalog is exported only inside its collection. Selected catalogs come
// first, in library order, then every listed catalog a selected collection
// references, once; each collection's scoped catalogs stay in its own list.
// An id that isn't one of profileID's own, or an empty selection, is
// ErrInvalidInput.
func (db *DB) ExportBundle(ctx context.Context, profileID uuid.UUID, catalogIDs, collectionIDs []uuid.UUID) (Bundle, error) {
	if len(catalogIDs)+len(collectionIDs) == 0 {
		return Bundle{}, fmt.Errorf("%w: select at least one catalog or collection to export", ErrInvalidInput)
	}
	listed, err := pickOwned(ctx, profileID, catalogIDs, "catalog", db.GetUserCatalogs, catalogID)
	if err != nil {
		return Bundle{}, err
	}
	trees, err := pickOwned(ctx, profileID, collectionIDs, "collection", db.GetUserCollections, treeID)
	if err != nil {
		return Bundle{}, err
	}
	return extractBundle(listed, trees, false), nil
}

func catalogID(c Catalog) uuid.UUID            { return c.ID }
func treeID(t CollectionWithFolders) uuid.UUID { return t.ID }

// pickOwned loads profileID's own items with load and picks those named by
// ids; see pickByID.
func pickOwned[T any](ctx context.Context, profileID uuid.UUID, ids []uuid.UUID, label string,
	load func(context.Context, uuid.UUID) ([]T, error), idOf func(T) uuid.UUID) ([]T, error) {
	own, err := load(ctx, profileID)
	if err != nil {
		return nil, err
	}
	return pickByID(own, ids, label, idOf)
}

// pickByID returns the items of own that ids names, in own's order and once
// each, or ErrInvalidInput naming the first id own doesn't hold.
func pickByID[T any](own []T, ids []uuid.UUID, label string, idOf func(T) uuid.UUID) ([]T, error) {
	index := indexByID(own, idOf)
	picked := make([]bool, len(own))
	for _, id := range ids {
		i, ok := index[id]
		if !ok {
			return nil, fmt.Errorf("%w: %s %s is not one of this profile's own", ErrInvalidInput, label, id)
		}
		picked[i] = true
	}
	out := make([]T, 0, len(ids))
	for i, item := range own {
		if picked[i] {
			out = append(out, item)
		}
	}
	return out, nil
}

// indexByID maps each item's id to its position in items.
func indexByID[T any](items []T, idOf func(T) uuid.UUID) map[uuid.UUID]int {
	index := make(map[uuid.UUID]int, len(items))
	for i, item := range items {
		index[idOf(item)] = i
	}
	return index
}

// ImportBundle writes b into profileID as new rows: each top-level catalog
// as a listed catalog and each collection with its folders, its own catalogs
// scoped to it. reuse maps a bundle catalog key, top-level or a collection's
// own, to one of profileID's listed catalogs, which that key's refs then
// point at in place of a new row. Returns the new listed catalogs and the
// new collections, each in bundle order.
//
// Every row id is minted here with uuid.New(); the file only ever supplies
// keys. The new rows are private and off the home screen, never linked, and
// each collection starts at version 1, never pushed. Titles are kept as
// they are. Importing one file twice gives two independent sets.
//
// b's params are in canonical form, which the caller puts them in when it
// checks the recipes (api.prepareBundle). Every check that needs no
// database runs before the transaction: Bundle.Validate, the reuse keys, and
// each row's form validator. Inside it, each reuse target must be one of
// profileID's listed catalogs. Any failure writes nothing.
func (db *DB) ImportBundle(ctx context.Context, profileID uuid.UUID, b Bundle, reuse map[string]uuid.UUID) ([]Catalog, []CollectionWithFolders, error) {
	plan, err := planImport(profileID, b, reuse)
	if err != nil {
		return nil, nil, err
	}
	var collectionIDs []uuid.UUID
	err = db.inTx(ctx, func(tx *sql.Tx) error {
		var err error
		collectionIDs, err = plan.write(ctx, tx, profileID)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return db.loadImported(ctx, plan.listedIDs(), collectionIDs)
}

// importPlan is what ImportBundle writes, fully checked except for the reuse
// targets: the new listed catalogs, the collection forms, and the reused
// catalog ids.
type importPlan struct {
	listed      []Catalog
	collections []CollectionForm
	reuseIDs    []uuid.UUID
}

// planImport checks b and reuse and builds the rows ImportBundle writes; see
// ImportBundle.
func planImport(profileID uuid.UUID, b Bundle, reuse map[string]uuid.UUID) (importPlan, error) {
	if err := b.Validate(); err != nil {
		return importPlan{}, err
	}
	if err := checkReuseKeys(b, reuse); err != nil {
		return importPlan{}, err
	}
	ids, listed, err := mintListed(profileID, b.Catalogs, reuse)
	if err != nil {
		return importPlan{}, err
	}
	collections, err := importForms(b.Collections, ids)
	if err != nil {
		return importPlan{}, err
	}
	return importPlan{listed: listed, collections: collections, reuseIDs: sortedValues(reuse)}, nil
}

// checkReuseKeys confirms every key reuse maps is a catalog key of b.
func checkReuseKeys(b Bundle, reuse map[string]uuid.UUID) error {
	keys := bundleKeys(b)
	for _, key := range slices.Sorted(maps.Keys(reuse)) {
		if !keys[key] {
			return fmt.Errorf("%w: reuse names catalog key %q, which the bundle doesn't have", ErrInvalidInput, key)
		}
	}
	return nil
}

// bundleKeys is the set of every catalog key in b.
func bundleKeys(b Bundle) map[string]bool {
	keys := map[string]bool{}
	for _, c := range b.Catalogs {
		keys[c.Key] = true
	}
	for _, bc := range b.Collections {
		for _, c := range bc.Catalogs {
			keys[c.Key] = true
		}
	}
	return keys
}

// sortedValues is reuse's catalog ids, ordered by key.
func sortedValues(reuse map[string]uuid.UUID) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(reuse))
	for _, key := range slices.Sorted(maps.Keys(reuse)) {
		ids = append(ids, reuse[key])
	}
	return ids
}

// mintListed builds a new listed catalog, owned by profileID, for each
// top-level catalog reuse doesn't map, held to the same rules as a catalog
// save. Returns every key's row id, reused and new alike, and the new rows in
// bundle order.
func mintListed(profileID uuid.UUID, catalogs []BundleCatalog, reuse map[string]uuid.UUID) (map[string]uuid.UUID, []Catalog, error) {
	ids := make(map[string]uuid.UUID, len(reuse)+len(catalogs))
	maps.Copy(ids, reuse)
	var listed []Catalog
	var problems []string
	for i, c := range catalogs {
		if _, reused := reuse[c.Key]; reused {
			continue
		}
		problems = append(problems, catalogSpecProblems(fmt.Sprintf("catalog %d: ", i), c.Type, c.Name, c.Provider, string(c.Params))...)
		row := importedCatalog(profileID, c)
		ids[c.Key] = row.ID
		listed = append(listed, row)
	}
	if len(problems) > 0 {
		return nil, nil, fmt.Errorf("%w: %s", ErrInvalidInput, strings.Join(problems, "; "))
	}
	return ids, listed, nil
}

// importedCatalog is c as a new private listed catalog owned by profileID,
// its name trimmed the way CatalogForm.normalized trims one.
func importedCatalog(profileID uuid.UUID, c BundleCatalog) Catalog {
	now := time.Now().UTC()
	return Catalog{
		ID:        uuid.New(),
		Type:      c.Type,
		Name:      strings.TrimSpace(c.Name),
		Provider:  c.Provider,
		Params:    string(c.Params),
		OwnerID:   profileID,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// importForms builds and validates the CollectionForm that writes each of
// collections. ids maps every top-level and every reused key to its row: a
// reused own catalog leaves the collection's own list, so its refs become
// references to the reused row like any top-level key's.
func importForms(collections []BundleCollection, ids map[string]uuid.UUID) ([]CollectionForm, error) {
	forms := make([]CollectionForm, len(collections))
	for i, bc := range collections {
		bc.Catalogs = withoutKeys(bc.Catalogs, ids)
		form := collectionFormFromBundle(bc, ids, false).normalized()
		for j := range form.Folders {
			form.Folders[j].Catalogs = dedupeCatalogRefs(form.Folders[j].Catalogs)
		}
		if err := form.Validate(); err != nil {
			return nil, fmt.Errorf("collection %d: %w", i, err)
		}
		forms[i] = form
	}
	return forms, nil
}

// withoutKeys is catalogs less every catalog whose key ids maps.
func withoutKeys(catalogs []BundleCatalog, ids map[string]uuid.UUID) []BundleCatalog {
	out := make([]BundleCatalog, 0, len(catalogs))
	for _, c := range catalogs {
		if _, ok := ids[c.Key]; !ok {
			out = append(out, c)
		}
	}
	return out
}

// dedupeCatalogRefs drops every ref to an existing catalog that repeats an
// earlier one's catalog and trimmed genre, keeping the first. Reuse can map
// two bundle catalogs onto one row, which would otherwise collide in the
// folder.
func dedupeCatalogRefs(refs []FolderCatalogRef) []FolderCatalogRef {
	seen := map[folderRefKey]bool{}
	out := make([]FolderCatalogRef, 0, len(refs))
	for _, ref := range refs {
		if ref.CatalogID != nil {
			key := folderRefKey{*ref.CatalogID, strings.TrimSpace(ref.Genre)}
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		out = append(out, ref)
	}
	return out
}

// write writes p inside tx: it checks each reuse target is one of
// profileID's listed catalogs, inserts the new listed catalogs, then creates
// each collection, all in bundle order. Returns the new collections' ids.
func (p importPlan) write(ctx context.Context, tx *sql.Tx, profileID uuid.UUID) ([]uuid.UUID, error) {
	if err := validateFolderRefs(ctx, tx, profileID, nil, p.reuseIDs); err != nil {
		return nil, err
	}
	if err := insertCatalogs(ctx, tx, p.listed); err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(p.collections))
	for i, form := range p.collections {
		created, _, err := createCollectionTx(ctx, tx, profileID, form)
		if err != nil {
			return nil, err
		}
		ids[i] = created.ID
	}
	return ids, nil
}

// insertCatalogs inserts each of catalogs in turn.
func insertCatalogs(ctx context.Context, tx *sql.Tx, catalogs []Catalog) error {
	for _, c := range catalogs {
		if _, err := insertCatalog(ctx, tx, c); err != nil {
			return err
		}
	}
	return nil
}

// listedIDs is the id of each new listed catalog, in bundle order.
func (p importPlan) listedIDs() []uuid.UUID {
	ids := make([]uuid.UUID, len(p.listed))
	for i, c := range p.listed {
		ids[i] = c.ID
	}
	return ids
}

// loadImported reads back the catalogs and collections an import wrote, each
// in the order of its ids.
func (db *DB) loadImported(ctx context.Context, catalogIDs, collectionIDs []uuid.UUID) ([]Catalog, []CollectionWithFolders, error) {
	catalogs, err := db.GetCatalogsByIDs(ctx, catalogIDs)
	if err != nil {
		return nil, nil, err
	}
	trees, err := db.GetCollectionsByIDs(ctx, collectionIDs)
	if err != nil {
		return nil, nil, err
	}
	return jsonwire.OrEmpty(inIDOrder(catalogs, catalogIDs, catalogID)), jsonwire.OrEmpty(inIDOrder(trees, collectionIDs, treeID)), nil
}

// inIDOrder sorts items into the order of ids.
func inIDOrder[T any](items []T, ids []uuid.UUID, idOf func(T) uuid.UUID) []T {
	pos := make(map[uuid.UUID]int, len(ids))
	for i, id := range ids {
		pos[id] = i
	}
	slices.SortFunc(items, func(a, b T) int { return cmp.Compare(pos[idOf(a)], pos[idOf(b)]) })
	return items
}
