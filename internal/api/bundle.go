package api

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/jsonwire"
	"github.com/hiidz/uno/internal/vault"
)

// maxBundleBodyBytes caps the body of the two import routes, which carry a
// whole library's worth of catalogs and collections rather than one save.
// An exported library is typically tens of kilobytes; this leaves generous
// room past that without letting a client make the server buffer for free.
// Its twin is MAX_BUNDLE_BYTES in web/src/features/bundle/text.ts, the
// largest text the import dialog sends; change one and you must change the
// other.
const maxBundleBodyBytes = 4 << 20 // 4 MiB

// prepareBundle runs the file-level checks on b, then checks every catalog
// recipe it carries with checkRecipe — the check a catalog save runs, since a
// bundle is client input like any other — and replaces each catalog's params
// with the canonical form its row will store. A recipe error names the
// catalog's key.
func (s *Server) prepareBundle(ctx context.Context, b *vault.Bundle) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if err := s.checkBundleCatalogs(ctx, b.Catalogs); err != nil {
		return err
	}
	for i := range b.Collections {
		if err := s.checkBundleCatalogs(ctx, b.Collections[i].Catalogs); err != nil {
			return err
		}
	}
	return nil
}

// checkBundleCatalogs is prepareBundle for one catalog list.
func (s *Server) checkBundleCatalogs(ctx context.Context, catalogs []vault.BundleCatalog) error {
	for i := range catalogs {
		c := &catalogs[i]
		params, err := s.checkRecipe(ctx, c.Type, c.Provider, string(c.Params))
		if err != nil {
			return fmt.Errorf("catalog %s: %w", c.Key, err)
		}
		c.Params = json.RawMessage(params)
	}
	return nil
}

// exportRequest is the body of POST .../export: the listed catalogs and the
// collections to export.
type exportRequest struct {
	CatalogIDs    []uuid.UUID `json:"catalog_ids"`
	CollectionIDs []uuid.UUID `json:"collection_ids"`
}

func (s *Server) exportBundle(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	var req exportRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	b, err := s.vault.ExportBundle(r.Context(), profileID, req.CatalogIDs, req.CollectionIDs)
	if err != nil {
		writeVaultError(w, "exportBundle", err, nil, "", "failed to export")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, b)
}

// importCheckRequest is the body of POST .../import/check.
type importCheckRequest struct {
	Bundle vault.Bundle `json:"bundle"`
}

// importCheck is the answer to POST .../import/check: what the bundle holds,
// in bundle order, for the import dialog to list before anything is written —
// its top-level catalogs, and each collection with its folders' titles and its
// own catalogs — each marked with what it matches in the caller's library. The
// dialog reads this rather than the bundle, whose format only the Go types
// define.
type importCheck struct {
	Catalogs    []checkCatalog    `json:"catalogs"`
	Collections []checkCollection `json:"collections"`
}

// checkCatalog is one bundle catalog as the dialog lists it, with its params
// in canonical form, the form a stored catalog's params take. Existing is
// every one of the caller's listed catalogs with the same recipe, the rows
// the import may reuse for it; it is empty when none has.
type checkCatalog struct {
	Key      string            `json:"key"`
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Params   string            `json:"params"`
	Existing []existingCatalog `json:"existing"`
}

// checkCollection is one bundle collection as the dialog lists it. Matched
// is whether its title, trimmed and in any case, is the title of one of the
// caller's own collections.
type checkCollection struct {
	Title    string         `json:"title"`
	Folders  []string       `json:"folders"`
	Matched  bool           `json:"matched"`
	Catalogs []checkCatalog `json:"catalogs"`
}

// existingCatalog is one of the caller's listed catalogs a checkCatalog
// may reuse.
type existingCatalog struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

func (s *Server) checkImport(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	var req importCheckRequest
	if !decodeJSONLimit(w, r, &req, maxBundleBodyBytes) {
		return
	}
	if err := s.prepareBundle(r.Context(), &req.Bundle); err != nil {
		writeVaultError(w, "checkImport", err, nil, "", "failed to check import")
		return
	}
	check, err := s.importCheckFor(r.Context(), profileID, req.Bundle)
	if err != nil {
		writeVaultError(w, "checkImport", err, nil, "", "failed to check import")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, check)
}

// importCheckFor is importCheckOf of b against profileID's own catalogs and
// collections.
func (s *Server) importCheckFor(ctx context.Context, profileID uuid.UUID, b vault.Bundle) (importCheck, error) {
	own, err := s.vault.GetUserCatalogs(ctx, profileID)
	if err != nil {
		return importCheck{}, err
	}
	ownCollections, err := s.vault.GetUserCollections(ctx, profileID)
	if err != nil {
		return importCheck{}, err
	}
	return importCheckOf(b, own, ownCollections), nil
}

// importCheckOf is the import check of b against own, the caller's listed
// catalogs, and ownCollections, the caller's collections. b's params are in
// canonical form (prepareBundle). Each catalog's existing catalogs are sorted
// by name, then id.
func importCheckOf(b vault.Bundle, own []vault.Catalog, ownCollections []vault.CollectionWithFolders) importCheck {
	byRecipe := existingByRecipe(own)
	titles := titleSet(ownCollections)
	check := importCheck{
		Catalogs:    checkCatalogs(b.Catalogs, byRecipe),
		Collections: make([]checkCollection, len(b.Collections)),
	}
	for i, bc := range b.Collections {
		check.Collections[i] = checkCollection{
			Title:    strings.TrimSpace(bc.Title),
			Folders:  folderTitles(bc.Folders),
			Matched:  titles[titleKey(bc.Title)],
			Catalogs: checkCatalogs(bc.Catalogs, byRecipe),
		}
	}
	return check
}

// checkCatalogs is each of catalogs as the import dialog lists it, with the
// existing catalogs of the same recipe from byRecipe.
func checkCatalogs(catalogs []vault.BundleCatalog, byRecipe map[string][]existingCatalog) []checkCatalog {
	out := make([]checkCatalog, len(catalogs))
	for i, c := range catalogs {
		existing := byRecipe[vault.RecipeHash(c.Type, c.Provider, string(c.Params))]
		out[i] = checkCatalog{
			Key: c.Key, Name: strings.TrimSpace(c.Name), Type: c.Type, Params: string(c.Params),
			Existing: jsonwire.OrEmpty(existing),
		}
	}
	return out
}

// folderTitles is the title of each of folders, trimmed, in order.
func folderTitles(folders []vault.BundleFolder) []string {
	titles := make([]string, len(folders))
	for i, f := range folders {
		titles[i] = strings.TrimSpace(f.Title)
	}
	return titles
}

// titleSet is the title of each of collections, as titleKey compares it.
func titleSet(collections []vault.CollectionWithFolders) map[string]bool {
	titles := make(map[string]bool, len(collections))
	for _, c := range collections {
		titles[titleKey(c.Title)] = true
	}
	return titles
}

// titleKey is the form two collection titles are compared in: trimmed, in
// lower case.
func titleKey(title string) string {
	return strings.ToLower(strings.TrimSpace(title))
}

// existingByRecipe groups own by recipe hash, each group sorted by
// name, then id.
func existingByRecipe(own []vault.Catalog) map[string][]existingCatalog {
	groups := make(map[string][]existingCatalog, len(own))
	for _, c := range own {
		groups[c.RecipeHash] = append(groups[c.RecipeHash], existingCatalog{ID: c.ID, Name: c.Name})
	}
	for _, group := range groups {
		slices.SortFunc(group, func(a, b existingCatalog) int {
			return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID.String(), b.ID.String()))
		})
	}
	return groups
}

// importRequest is the body of POST .../import: the bundle, the bundle
// catalog keys to point at one of the caller's listed catalogs instead of
// importing (see vault.DB.ImportBundle), and the positions of the bundle's
// collections to leave out, each with its own catalogs.
type importRequest struct {
	Bundle          vault.Bundle         `json:"bundle"`
	Reuse           map[string]uuid.UUID `json:"reuse"`
	SkipCollections []int                `json:"skip_collections"`
}

// importResult is the answer to POST .../import: the new listed catalogs
// and the new collections. A reused catalog is not among them.
type importResult struct {
	Catalogs    []vault.Catalog               `json:"catalogs"`
	Collections []vault.CollectionWithFolders `json:"collections"`
}

// importBundle checks the bundle afresh rather than trusting an earlier
// import/check, then writes it.
func (s *Server) importBundle(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	var req importRequest
	if !decodeJSONLimit(w, r, &req, maxBundleBodyBytes) {
		return
	}
	b, err := s.importedBundle(r.Context(), req)
	if err != nil {
		writeVaultError(w, "importBundle", err, nil, "", "failed to import")
		return
	}
	catalogs, collections, err := s.vault.ImportBundle(r.Context(), profileID, b, req.Reuse)
	if err != nil {
		writeVaultError(w, "importBundle", err, nil, "", "failed to import")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, importResult{Catalogs: catalogs, Collections: collections})
}

// importedBundle is the bundle req writes: its skipped collections left out,
// then the rest prepared by prepareBundle.
func (s *Server) importedBundle(ctx context.Context, req importRequest) (vault.Bundle, error) {
	b, err := withoutCollections(req.Bundle, req.SkipCollections)
	if err != nil {
		return vault.Bundle{}, err
	}
	if err := s.prepareBundle(ctx, &b); err != nil {
		return vault.Bundle{}, err
	}
	return b, nil
}

// withoutCollections is b less the collections at the positions skip names,
// each leaving with its own catalogs. A position b doesn't have is
// ErrInvalidInput.
func withoutCollections(b vault.Bundle, skip []int) (vault.Bundle, error) {
	drop, err := skipSet(skip, len(b.Collections))
	if err != nil {
		return vault.Bundle{}, err
	}
	kept := make([]vault.BundleCollection, 0, len(b.Collections))
	for i, bc := range b.Collections {
		if !drop[i] {
			kept = append(kept, bc)
		}
	}
	b.Collections = kept
	return b, nil
}

// skipSet is skip as a set of positions, each one of n collections.
func skipSet(skip []int, n int) (map[int]bool, error) {
	drop := make(map[int]bool, len(skip))
	for _, i := range skip {
		if i < 0 || i >= n {
			return nil, fmt.Errorf("%w: skip_collections names collection %d, which isn't one of the bundle's %d", vault.ErrInvalidInput, i, n)
		}
		drop[i] = true
	}
	return drop, nil
}
