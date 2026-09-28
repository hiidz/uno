package api

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/vault"
)

// maxBundleBodyBytes caps the body of the two import routes, which carry a
// whole library's worth of catalogs and collections rather than one save.
// An exported library is typically tens of kilobytes; this leaves generous
// room past that without letting a client make the server buffer for free.
// Its twin is MAX_BUNDLE_BYTES in web/src/features/bundle/ImportDialog.tsx,
// the largest file the import dialog reads; change one and you must change
// the other.
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

// importCheck is the answer to POST .../import/check: how many catalogs
// (top-level and in collections), collections and folders the bundle holds,
// and every catalog in it that matches one of the caller's listed catalogs.
type importCheck struct {
	Catalogs    int           `json:"catalogs"`
	Collections int           `json:"collections"`
	Folders     int           `json:"folders"`
	Matches     []importMatch `json:"matches"`
}

// importMatch is one bundle catalog whose recipe equals that of one or more
// of the caller's listed catalogs, which are the rows the import may reuse
// for it. Scope is "listed" for a top-level catalog, with an empty
// Collection, or "scoped" for one of a collection's own, with Collection
// that collection's title.
type importMatch struct {
	Key        string            `json:"key"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Scope      string            `json:"scope"`
	Collection string            `json:"collection"`
	Existing   []existingCatalog `json:"existing"`
}

// existingCatalog is one of the caller's listed catalogs an importMatch
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
	own, err := s.vault.GetUserCatalogs(r.Context(), profileID)
	if err != nil {
		writeVaultError(w, "checkImport", err, nil, "", "failed to check import")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, importCheckOf(req.Bundle, own))
}

// importCheckOf is the import check of b against own, the caller's listed
// catalogs. b's params are in canonical form (prepareBundle). Matches
// follow bundle order, and each match's existing catalogs are sorted by
// name, then id.
func importCheckOf(b vault.Bundle, own []vault.Catalog) importCheck {
	byRecipe := existingByRecipe(own)
	check := importCheck{
		Catalogs:    len(b.Catalogs),
		Collections: len(b.Collections),
		Matches:     appendMatches([]importMatch{}, b.Catalogs, "listed", "", byRecipe),
	}
	for _, bc := range b.Collections {
		check.Catalogs += len(bc.Catalogs)
		check.Folders += len(bc.Folders)
		check.Matches = appendMatches(check.Matches, bc.Catalogs, "scoped", bc.Title, byRecipe)
	}
	return check
}

// appendMatches appends an importMatch for each of catalogs that has an
// existing catalog.
func appendMatches(matches []importMatch, catalogs []vault.BundleCatalog, scope, collection string, byRecipe map[string][]existingCatalog) []importMatch {
	for _, c := range catalogs {
		if existing := byRecipe[vault.RecipeHash(c.Type, c.Provider, string(c.Params))]; len(existing) > 0 {
			matches = append(matches, importMatch{
				Key: c.Key, Name: c.Name, Type: c.Type, Scope: scope, Collection: collection, Existing: existing,
			})
		}
	}
	return matches
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

// importRequest is the body of POST .../import: the bundle, and the bundle
// catalog keys to point at one of the caller's listed catalogs instead of
// importing (see vault.DB.ImportBundle).
type importRequest struct {
	Bundle vault.Bundle         `json:"bundle"`
	Reuse  map[string]uuid.UUID `json:"reuse"`
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
	if err := s.prepareBundle(r.Context(), &req.Bundle); err != nil {
		writeVaultError(w, "importBundle", err, nil, "", "failed to import")
		return
	}
	catalogs, collections, err := s.vault.ImportBundle(r.Context(), profileID, req.Bundle, req.Reuse)
	if err != nil {
		writeVaultError(w, "importBundle", err, nil, "", "failed to import")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, importResult{Catalogs: catalogs, Collections: collections})
}
