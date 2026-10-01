// What's waiting for a push: the difference between what Nuvio holds (the
// push record) and what a push of the Home as Uno stores it would send now.
// Unpushed Home edits live in the builder's tab and are the SPA's to list.

package vault

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/jsonwire"
)

// Pending kinds and changes.
const (
	pendingCatalog    = "catalog"
	pendingCollection = "collection"

	// PendingAdded is a row on Home that Nuvio holds nothing for.
	PendingAdded = "added"
	// PendingChanged is a row Nuvio holds differently from how a push would
	// send it now. A collection counts as changed when what a push sends for
	// it differs, or when a catalog its folders use does.
	PendingChanged = "changed"
	// PendingRemoved is a row Nuvio holds that Uno no longer has: deleted
	// since the last push, which drops it.
	PendingRemoved = "removed"
)

// PendingChange is one row a push would change in Nuvio. Name is the row's
// name as a push would send it, or as the last push left it for a removed one.
type PendingChange struct {
	Kind   string    `json:"kind"`
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	Change string    `json:"change"`
}

// PendingPush is what a push of profileID's Home, as the Home columns hold it,
// would change in Nuvio: catalog rows first, then collections, each as
// pendingDiff orders them. Both sides are read in one snapshot.
func (db *DB) PendingPush(ctx context.Context, profileID uuid.UUID) ([]PendingChange, error) {
	tx, err := db.conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // read-only; never committed

	held, _, err := heldRecord(ctx, tx, profileID)
	if err != nil {
		return nil, err
	}
	now, err := StoredPushRecord(ctx, tx, profileID)
	if err != nil {
		return nil, err
	}
	return pendingDiff(held, now)
}

// pendingDiff is held's difference with now, both push records.
func pendingDiff(held, now PushRecord) ([]PendingChange, error) {
	catalogs := catalogDiff(held, now)
	collections, err := collectionDiff(held, now)
	if err != nil {
		return nil, err
	}
	return jsonwire.OrEmpty(append(catalogs, collections...)), nil
}

// catalogKey is a catalog as a push sends it, which two catalogs are the same
// by: name, type, provider and params, compacted.
func catalogKey(c PushedCatalog) string {
	raw, _ := json.Marshal(c) // a PushedCatalog always marshals
	return string(raw)
}

func catalogsByID(catalogs []PushedCatalog) map[uuid.UUID]PushedCatalog {
	byID := make(map[uuid.UUID]PushedCatalog, len(catalogs))
	for _, c := range catalogs {
		byID[c.ID] = c
	}
	return byID
}

// catalogDiff is the changes to the catalogs with a Home row of their own:
// each one in now's Home that is new to Nuvio or differs from what it holds,
// in Home order, then each one Nuvio holds that is gone from now's Home.
func catalogDiff(held, now PushRecord) []PendingChange {
	heldHome := make(map[uuid.UUID]bool, len(held.Home.Catalogs))
	for _, c := range held.Home.Catalogs {
		heldHome[c.CatalogID] = true
	}
	nowHome := make(map[uuid.UUID]bool, len(now.Home.Catalogs))
	for _, c := range now.Home.Catalogs {
		nowHome[c.CatalogID] = true
	}
	heldCatalogs, nowCatalogs := catalogsByID(held.Catalogs), catalogsByID(now.Catalogs)

	var changes []PendingChange
	for _, c := range now.Home.Catalogs {
		cur := nowCatalogs[c.CatalogID]
		switch was, ok := heldCatalogs[c.CatalogID]; {
		case !heldHome[c.CatalogID] || !ok:
			changes = append(changes, PendingChange{pendingCatalog, c.CatalogID, cur.Name, PendingAdded})
		case catalogKey(was) != catalogKey(cur):
			changes = append(changes, PendingChange{pendingCatalog, c.CatalogID, cur.Name, PendingChanged})
		}
	}
	for _, c := range held.Home.Catalogs {
		if !nowHome[c.CatalogID] {
			changes = append(changes, PendingChange{pendingCatalog, c.CatalogID, heldCatalogs[c.CatalogID].Name, PendingRemoved})
		}
	}
	return changes
}

// collectionDiff is the changes to the collections: each one in now that is
// new to Nuvio or differs from what it holds, in Home order, then each one
// Nuvio holds that is gone from now.
func collectionDiff(held, now PushRecord) ([]PendingChange, error) {
	heldBytes := make(map[uuid.UUID]json.RawMessage, len(held.Collections))
	heldHeads := make([]pushedHead, len(held.Collections))
	for i, raw := range held.Collections {
		head, err := pushedHeadOf(raw)
		if err != nil {
			return nil, err
		}
		heldHeads[i], heldBytes[head.ID] = head, raw
	}
	changedCatalogs := changedCatalogIDs(held, now)

	var changes []PendingChange
	nowIDs := make(map[uuid.UUID]bool, len(now.Collections))
	for _, raw := range now.Collections {
		head, err := pushedHeadOf(raw)
		if err != nil {
			return nil, err
		}
		nowIDs[head.ID] = true
		if change, ok := collectionChange(head, raw, heldBytes, changedCatalogs); ok {
			changes = append(changes, change)
		}
	}
	for _, head := range heldHeads {
		if !nowIDs[head.ID] {
			changes = append(changes, PendingChange{pendingCollection, head.ID, head.Title, PendingRemoved})
		}
	}
	return changes, nil
}

// collectionChange is how a push would change the collection it sends as raw,
// given heldBytes, each collection the last push sent, and the catalogs whose
// entries changed: not at all when ok is false.
func collectionChange(head pushedHead, raw json.RawMessage, heldBytes map[uuid.UUID]json.RawMessage, changedCatalogs map[uuid.UUID]bool) (change PendingChange, ok bool) {
	was, held := heldBytes[head.ID]
	switch {
	case !held:
		return PendingChange{pendingCollection, head.ID, head.Title, PendingAdded}, true
	case !bytes.Equal(was, raw) || usesAny(raw, changedCatalogs):
		return PendingChange{pendingCollection, head.ID, head.Title, PendingChanged}, true
	}
	return PendingChange{}, false
}

// changedCatalogIDs is every catalog both records reach whose entry differs:
// renamed, or its recipe edited.
func changedCatalogIDs(held, now PushRecord) map[uuid.UUID]bool {
	heldCatalogs := catalogsByID(held.Catalogs)
	changed := make(map[uuid.UUID]bool)
	for _, c := range now.Catalogs {
		if was, ok := heldCatalogs[c.ID]; ok && catalogKey(was) != catalogKey(c) {
			changed[c.ID] = true
		}
	}
	return changed
}

// usesAny reports whether the collection a push sends as raw has a folder
// source naming one of catalogs.
func usesAny(raw json.RawMessage, catalogs map[uuid.UUID]bool) bool {
	if len(catalogs) == 0 {
		return false
	}
	var sent PushCollection
	if err := json.Unmarshal(raw, &sent); err != nil {
		return false
	}
	for _, f := range sent.Folders {
		for _, src := range f.CatalogSources {
			_, rawID, ok := strings.Cut(src.CatalogID, "-")
			if id, err := uuid.Parse(rawID); ok && err == nil && catalogs[id] {
				return true
			}
		}
	}
	return false
}
