package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/addon"
	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/vault"
)

// pushRequest is POST /api/p/{i}/push's body: the full pending home-screen
// selection, carried in one call rather than standalone PUT .../selection
// endpoints — selection is only ever written here, in one transaction,
// after Nuvio has accepted the push. See the "HTTP surface" section of
// docs/architecture.md.
type pushRequest struct {
	Catalogs    vault.CatalogSelectionForm    `json:"catalogs"`
	Collections vault.CollectionSelectionForm `json:"collections"`
}

// pushResult is the always-JSON response, past auth/profile resolution.
// Flat by design — success or failure, not partial-progress flags — since
// push's ordering (validate → Nuvio → local write) guarantees an ordinary
// failure means nothing changed. UndoFailed marks the one case that
// guarantee doesn't cover: see the final commit step below.
type pushResult struct {
	Success     bool   `json:"success"`
	ManifestURL string `json:"manifest_url,omitempty"`
	Error       string `json:"error,omitempty"`
	UndoFailed  bool   `json:"undo_failed,omitempty"`
}

// push serves POST /api/p/{profileIndex}/push: an explicit, user-triggered
// sync of this profile's manifest URL and collections into Nuvio, carrying
// the full pending selection in its body.
//
// Ordering is Nuvio-first, local-write-last: validate, push addons, push
// collections, and only then commit the selection to Uno's own vault. This
// avoids holding a SQLite write transaction open across several sequential
// Nuvio HTTP calls, and means an ordinary failure leaves nothing written on
// either side.
//
// Addons before collections: a pushed collection's catalogSources reference
// this addon's manifest id, so installing the addon first means a client
// that reads collections right after a push already has something to
// resolve those references against. If addons push fails, collections is
// never attempted.
func (s *Server) push(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	profileID, ok := profileIDFrom(ctx)
	if !ok {
		http.Error(w, "missing profile", http.StatusUnauthorized)
		return
	}
	accessToken, ok := nuvioTokenFrom(ctx)
	if !ok {
		http.Error(w, "missing nuvio token", http.StatusUnauthorized)
		return
	}

	profile, err := s.vault.GetProfileByID(ctx, profileID)
	if err != nil {
		http.Error(w, "failed to resolve profile", http.StatusInternalServerError)
		return
	}

	var body pushRequest
	if !decodeJSON(w, r, &body) {
		return
	}

	manifestURL := s.siteBaseURL + addon.ManifestPath(profile.Token)

	catalogIDs := make([]uuid.UUID, len(body.Catalogs.Catalogs))
	for i, c := range body.Catalogs.Catalogs {
		catalogIDs[i] = c.CatalogID
	}

	// Load-bearing, not a fail-fast nicety: with the write moved to the end,
	// this is the only check standing between the request body and a
	// third-party API call.
	if err := s.vault.ValidateSelectionAccess(ctx, profileID, catalogIDs, body.Collections.CollectionIDs); err != nil {
		log.Printf("push: validation failed: %v", err)
		status := http.StatusInternalServerError
		if errors.Is(err, vault.ErrInvalidInput) {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, pushResult{Error: "push failed"})
		return
	}

	if err := s.pushAddons(ctx, accessToken, profile.NuvioProfileIndex, manifestURL); err != nil {
		log.Printf("push: addons push failed: %v", err)
		writeJSON(w, nuvioErrorStatus(err), pushResult{Error: "push failed"})
		return
	}

	pulled, collectionVersions, err := s.pushCollections(ctx, accessToken, profile.NuvioProfileIndex, profileID, body.Collections.CollectionIDs)
	if err != nil {
		log.Printf("push: collections push failed: %v", err)
		writeJSON(w, nuvioErrorStatus(err), pushResult{Error: "push failed"})
		return
	}

	if err := s.vault.SaveSelectionsForPush(ctx, profileID, body.Catalogs, body.Collections, collectionVersions); err != nil {
		log.Printf("push: local commit failed after nuvio succeeded, reverting collections: %v", err)
		if revertErr := s.nuvio.PushCollections(ctx, accessToken, profile.NuvioProfileIndex, pulled); revertErr != nil {
			log.Printf("push: compensating revert also failed: %v", revertErr)
			writeJSON(w, http.StatusInternalServerError, pushResult{Error: "push failed", UndoFailed: true})
			return
		}
		writeJSON(w, http.StatusInternalServerError, pushResult{Error: "push failed"})
		return
	}

	writeJSON(w, http.StatusOK, pushResult{Success: true, ManifestURL: manifestURL})
}

// pushAddons runs the addons read-modify-write cycle: pull the profile's
// current addons, upsert Uno's own manifest URL into that list by URL
// match (Nuvio's own dedup key — see the "Push" section of
// docs/architecture.md), and push the complete merged list back —
// omitting any existing addon would delete it.
// Has no dependency on the pending selection, so it's unaffected by push's
// Nuvio-first ordering.
func (s *Server) pushAddons(ctx context.Context, accessToken string, nuvioProfileIndex int, manifestURL string) error {
	current, err := s.nuvio.ListAddons(ctx, accessToken, nuvioProfileIndex)
	if err != nil {
		return err
	}

	merged := make([]nuvio.PushAddonInput, len(current))
	found := false
	for i, a := range current {
		merged[i] = nuvio.PushAddonInput{URL: a.URL, Name: a.Name, Enabled: a.Enabled, SortOrder: a.SortOrder}
		if a.URL == manifestURL {
			merged[i].Name = addon.Name
			merged[i].Enabled = true
			found = true
		}
	}
	if !found {
		merged = append(merged, nuvio.PushAddonInput{
			URL: manifestURL, Name: addon.Name, Enabled: true, SortOrder: len(current),
		})
	}

	return s.nuvio.PushAddons(ctx, accessToken, nuvioProfileIndex, merged)
}

// pulledCollection is the subset of a pulled Nuvio collection's fields push's
// merge needs to decide whether to drop it: its id, for the owned-set match,
// and each folder's sources, for the addon-id heuristic below (isUnoManaged).
// Confirmed against a real Nuvio profile: a collection Uno has pushed
// round-trips its folder sources under "catalogSources", the
// same key Uno's own push writes (and the name the public doc documents —
// see the "Push wire shape" section of docs/data-model.md). "sources" is
// still parsed too, defensively, in case a Nuvio-native collection (never
// pushed by Uno) uses it instead — that case wasn't exercised by this check.
type pulledCollection struct {
	ID      string `json:"id"`
	Folders []struct {
		Sources        []pulledSource `json:"sources"`
		CatalogSources []pulledSource `json:"catalogSources"`
	} `json:"folders"`
}

type pulledSource struct {
	AddonID string `json:"addonId"`
}

// isUnoManaged reports whether every source in every folder of a pulled
// collection carries this addon's id — the signal that Uno once pushed the
// collection but no longer knows its id (hard-deleted locally, or from a
// recreated database). Nothing but Uno's own push ever writes a source
// pointing at addon.ID, so this has nowhere else to come from. A collection
// with no sources at all (no folders, or folders with none) is not treated
// as Uno-managed — there's nothing to match on, and a false positive here
// would silently delete a Nuvio-native collection from the pushed blob.
func isUnoManaged(c pulledCollection) bool {
	found := false
	for _, f := range c.Folders {
		for _, src := range append(f.Sources, f.CatalogSources...) {
			found = true
			if src.AddonID != addon.ID {
				return false
			}
		}
	}
	return found
}

// pushCollections runs the collections read-modify-write cycle, sourcing
// the pending selection from the request body rather than reading it back
// out of the vault — the local write hasn't happened yet at this point in
// push's sequence (see push above). Nuvio's blob is full-replace and holds
// collections Uno knows nothing about (its own native UI, or another
// client), so the merge has to touch only what Uno manages and leave
// everything else byte-for-byte untouched:
//
//  1. Pull the current blob as raw JSON per element — never decoded into a
//     generic map, which would round-trip numbers through float64 and
//     silently corrupt any collection Uno doesn't own.
//  2. Drop every pulled entry that is either owned by this profile, or
//     Uno-managed by the addon-id heuristic (isUnoManaged). With the closed
//     graph, everything selected is owned, so the owned set alone is what
//     the old selection-union used to be for; the heuristic covers the
//     residual case that union existed for — a collection Uno once pushed
//     but has since forgotten (hard-deleted, or from a recreated database)
//     — without needing to read the profile's previous selection at all.
//  3. Append freshly built entries for the profile's pending selection.
//
// Returns the pulled blob on success so push can use it for a compensating
// revert if the local commit that follows this call ends up failing, plus
// the version of each selected collection as read here — never re-read
// later, since that's the version Nuvio was actually sent (see
// vault.SaveSelectionsForPush's pushed_version stamp).
func (s *Server) pushCollections(ctx context.Context, accessToken string, nuvioProfileIndex int, profileID uuid.UUID, orderedCollectionIDs []uuid.UUID) ([]json.RawMessage, map[uuid.UUID]int, error) {
	pulled, err := s.nuvio.PullCollections(ctx, accessToken, nuvioProfileIndex)
	if err != nil {
		return nil, nil, err
	}

	ownedIDs, err := s.vault.GetOwnedCollectionIDs(ctx, profileID)
	if err != nil {
		return nil, nil, fmt.Errorf("loading owned collections: %w", err)
	}
	selected, err := s.vault.GetCollectionsByIDs(ctx, orderedCollectionIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("loading pending collection selection: %w", err)
	}
	selected = reorderCollections(selected, orderedCollectionIDs)

	versions := make(map[uuid.UUID]int, len(selected))
	for _, c := range selected {
		versions[c.ID] = c.Version
	}

	ownedByID := make(map[string]bool, len(ownedIDs))
	for _, id := range ownedIDs {
		ownedByID[id.String()] = true
	}

	var allCatalogIDs []uuid.UUID
	for _, c := range selected {
		for _, f := range c.Folders {
			allCatalogIDs = append(allCatalogIDs, f.CatalogIDs...)
		}
	}
	catalogs, err := s.vault.GetCatalogsByIDs(ctx, allCatalogIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("loading referenced catalogs: %w", err)
	}
	catalogsByID := make(map[uuid.UUID]vault.Catalog, len(catalogs))
	for _, c := range catalogs {
		catalogsByID[c.ID] = c
	}

	kept := make([]json.RawMessage, 0, len(pulled)+len(selected))
	for _, raw := range pulled {
		var parsed pulledCollection
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, nil, fmt.Errorf("parsing pulled collection: %w", err)
		}
		if ownedByID[parsed.ID] || isUnoManaged(parsed) {
			continue
		}
		kept = append(kept, raw)
	}

	for _, c := range selected {
		rawFresh, err := json.Marshal(buildPushCollection(c, catalogsByID))
		if err != nil {
			return nil, nil, fmt.Errorf("marshaling collection for push: %w", err)
		}
		kept = append(kept, rawFresh)
	}

	if err := s.nuvio.PushCollections(ctx, accessToken, nuvioProfileIndex, kept); err != nil {
		return nil, nil, err
	}
	return pulled, versions, nil
}

// reorderCollections sorts collections to match orderedIDs.
// GetCollectionsByIDs queries by a plain IN clause and doesn't preserve
// input order, but push needs the client's actual ordering to build the
// pushed collections in the right sequence.
func reorderCollections(collections []vault.CollectionWithFolders, orderedIDs []uuid.UUID) []vault.CollectionWithFolders {
	byID := make(map[uuid.UUID]vault.CollectionWithFolders, len(collections))
	for _, c := range collections {
		byID[c.ID] = c
	}
	ordered := make([]vault.CollectionWithFolders, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		if c, ok := byID[id]; ok {
			ordered = append(ordered, c)
		}
	}
	return ordered
}

// buildPushCollection converts one of Uno's own collections from its
// snake_case DB shape into push/pull's camelCase wire shape, resolving each
// folder's catalog_ids into addonId/type/catalogId triples via catalogsByID.
// A catalog id missing from that map (a dangling ref) is skipped rather than
// failing the whole push — see addon.ManifestID for the id scheme shared
// with the addon manifest.
func buildPushCollection(c vault.CollectionWithFolders, catalogsByID map[uuid.UUID]vault.Catalog) nuvio.PushCollection {
	folders := make([]nuvio.PushFolder, len(c.Folders))
	for i, f := range c.Folders {
		sources := make([]nuvio.CatalogSource, 0, len(f.CatalogIDs))
		for _, catalogID := range f.CatalogIDs {
			catalog, ok := catalogsByID[catalogID]
			if !ok {
				continue
			}
			sources = append(sources, nuvio.CatalogSource{
				AddonID:   addon.ID,
				Type:      catalog.Type,
				CatalogID: addon.ManifestID(catalog),
			})
		}
		folders[i] = nuvio.PushFolder{
			ID:             f.ID.String(),
			Title:          f.Title,
			CoverImageURL:  f.CoverImageURL,
			CoverEmoji:     f.CoverEmoji,
			TileShape:      f.TileShape,
			HideTitle:      f.HideTitle,
			CatalogSources: sources,
		}
	}

	return nuvio.PushCollection{
		ID:               c.ID.String(),
		Title:            c.Title,
		BackdropImageURL: c.BackdropImageURL,
		PinToTop:         c.PinToTop,
		ViewMode:         c.ViewMode,
		ShowAllTab:       c.ShowAllTab,
		Folders:          folders,
	}
}

// nuvioErrorStatus classifies a push-stage error into a status code: a
// failed Nuvio call itself is 502 (upstream problem), anything else (a
// vault read failing, JSON parsing a pulled collection) is 500.
func nuvioErrorStatus(err error) int {
	if errors.Is(err, nuvio.ErrNuvioRequestFailed) {
		return http.StatusBadGateway
	}
	return http.StatusInternalServerError
}
