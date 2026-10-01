// The push record: what a profile's last push put in Nuvio, kept whole in
// push_records — each collection as the bytes sent, the Home selection, and
// every catalog Nuvio can reach with its name, type and params inline. One
// builder makes it (buildPushRecord): push builds it before contacting Nuvio,
// sends its collections, and stores it once Nuvio has accepted them.

package vault

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/jsonwire"
)

// PushRecord is what one push puts in Nuvio for a profile.
type PushRecord struct {
	// Collections is each pushed collection as the exact bytes push sends for
	// it (PushJSON), in Home order.
	Collections []json.RawMessage `json:"collections"`
	// Home is the Home selection the push carries.
	Home PushedHome `json:"home"`
	// Catalogs is every catalog Nuvio can reach: those with a Home row of
	// their own, in Home order, then those only the collections' folders use,
	// in the order the collections, their folders and their refs list them.
	Catalogs []PushedCatalog `json:"catalogs"`
}

// PushedHome is a push's Home selection: catalog rows with Home or Discover,
// and collections with Show first, each in Home order.
type PushedHome struct {
	Catalogs    []SelectedCatalogInput    `json:"catalogs"`
	Collections []SelectedCollectionInput `json:"collections"`
}

// PushedCatalog is one catalog Nuvio can reach, as a push left it.
type PushedCatalog struct {
	ID       uuid.UUID       `json:"id"`
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Provider string          `json:"provider"`
	Params   json.RawMessage `json:"params"`
}

// BuildPushRecord is what a push of catalogs and collections, profileID's
// pending Home selection, puts in Nuvio now. Push calls it after
// ValidateSelectionAccess and before contacting Nuvio.
func (db *DB) BuildPushRecord(ctx context.Context, profileID uuid.UUID, catalogs CatalogSelectionForm, collections CollectionSelectionForm) (PushRecord, error) {
	return buildPushRecord(ctx, db.conn, profileID, catalogs, collections)
}

// StoredPushRecord is what a push of profileID's Home as push last stored it
// (the Home columns) puts in Nuvio now, read through tx: the v5→v6
// migration's backfill.
func StoredPushRecord(ctx context.Context, tx *sql.Tx, profileID uuid.UUID) (PushRecord, error) {
	listed, err := selectLeanCatalogs(ctx, tx, "c.owner_id = ? AND c.home_sort_order IS NOT NULL", profileID.String())
	if err != nil {
		return PushRecord{}, err
	}
	slices.SortFunc(listed, compareByHomeSortOrder)
	var catalogs CatalogSelectionForm
	for _, c := range listed {
		catalogs.Catalogs = append(catalogs.Catalogs, SelectedCatalogInput{CatalogID: c.ID, ShowInHome: c.ShowInHome})
	}

	onHome, err := selectLeanCollections(ctx, tx, "col.owner_id = ? AND col.home_sort_order IS NOT NULL", profileID.String())
	if err != nil {
		return PushRecord{}, err
	}
	slices.SortFunc(onHome, compareCollectionsByHomeSortOrder)
	var collections CollectionSelectionForm
	for _, c := range onHome {
		collections.Collections = append(collections.Collections, SelectedCollectionInput{CollectionID: c.ID, PinToTop: c.PinToTop})
	}
	return buildPushRecord(ctx, tx, profileID, catalogs, collections)
}

// buildPushRecord is the push record of catalogs and collections, profileID's
// Home selection, read through q.
func buildPushRecord(ctx context.Context, q querier, profileID uuid.UUID, catalogs CatalogSelectionForm, collections CollectionSelectionForm) (PushRecord, error) {
	trees, err := selectedTrees(ctx, q, profileID, collections)
	if err != nil {
		return PushRecord{}, err
	}
	listed, err := selectedCatalogs(ctx, q, profileID, catalogs)
	if err != nil {
		return PushRecord{}, err
	}

	record := PushRecord{
		Collections: make([]json.RawMessage, 0, len(trees)),
		Home: PushedHome{
			Catalogs:    jsonwire.OrEmpty(catalogs.Catalogs),
			Collections: jsonwire.OrEmpty(collections.Collections),
		},
		Catalogs: reachableCatalogs(listed, trees),
	}
	for _, tree := range trees {
		raw, err := tree.PushJSON()
		if err != nil {
			return PushRecord{}, fmt.Errorf("marshaling collection for push: %w", err)
		}
		record.Collections = append(record.Collections, raw)
	}
	return record, nil
}

// selectedTrees is profileID's collections selection names, each with its
// tree, in selection order and with the pin its entry carries, read through
// q.
func selectedTrees(ctx context.Context, q querier, profileID uuid.UUID, selection CollectionSelectionForm) ([]CollectionWithFolders, error) {
	ids := selection.CollectionIDs()
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := selectLeanCollections(ctx, q, "col.id IN (SELECT value FROM json_each(?)) AND col.owner_id = ?", idsJSON(ids), profileID.String())
	if err != nil {
		return nil, err
	}
	trees, err := assembleCollectionTree(ctx, q, rows, leanCatalogsByIDs)
	if err != nil {
		return nil, err
	}
	return applySelection(trees, selection), nil
}

// applySelection sorts trees to match selection's order and gives each the
// pin its selection entry carries: the pin a push sends is the pending one,
// which only the write after Nuvio accepted the push stores.
func applySelection(trees []CollectionWithFolders, selection CollectionSelectionForm) []CollectionWithFolders {
	byID := make(map[uuid.UUID]CollectionWithFolders, len(trees))
	for _, t := range trees {
		byID[t.ID] = t
	}
	ordered := make([]CollectionWithFolders, 0, len(selection.Collections))
	for _, entry := range selection.Collections {
		if t, ok := byID[entry.CollectionID]; ok {
			t.PinToTop = entry.PinToTop
			ordered = append(ordered, t)
		}
	}
	return ordered
}

// selectedCatalogs is profileID's listed catalogs selection names, in
// selection order, read through q.
func selectedCatalogs(ctx context.Context, q querier, profileID uuid.UUID, selection CatalogSelectionForm) ([]Catalog, error) {
	if len(selection.Catalogs) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(selection.Catalogs))
	for i, c := range selection.Catalogs {
		ids[i] = c.CatalogID
	}
	rows, err := selectLeanCatalogs(ctx, q, "c.id IN (SELECT value FROM json_each(?)) AND c.owner_id = ? AND c.collection_id IS NULL",
		idsJSON(ids), profileID.String())
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]Catalog, len(rows))
	for _, c := range rows {
		byID[c.ID] = c
	}
	ordered := make([]Catalog, 0, len(ids))
	for _, id := range ids {
		if c, ok := byID[id]; ok {
			ordered = append(ordered, c)
		}
	}
	return ordered, nil
}

// reachableCatalogs is every catalog Nuvio can reach once listed, in Home
// order, and trees are on Home: listed first, then each catalog only the
// trees use, once.
func reachableCatalogs(listed []Catalog, trees []CollectionWithFolders) []PushedCatalog {
	seen := make(map[uuid.UUID]bool)
	reachable := []PushedCatalog{}
	add := func(c Catalog) {
		if seen[c.ID] {
			return
		}
		seen[c.ID] = true
		reachable = append(reachable, PushedCatalog{ID: c.ID, Name: c.Name, Type: c.Type, Provider: c.Provider, Params: json.RawMessage(c.Params)})
	}
	for _, c := range listed {
		add(c)
	}
	for _, tree := range trees {
		for _, c := range tree.Catalogs {
			add(c)
		}
	}
	return reachable
}

// SavePush stores record as profileID's push record, with the Home selection
// it carries, in one transaction: the local half of push, run only after
// Nuvio accepted what record holds (internal/api/push.go). record is what
// push built before contacting Nuvio, never the rows as they stand now, so a
// save landing during the push still shows as waiting for the next one.
func (db *DB) SavePush(ctx context.Context, profileID uuid.UUID, record PushRecord) error {
	return db.inTx(ctx, func(tx *sql.Tx) error {
		if err := saveCatalogSelectionTx(ctx, tx, profileID, CatalogSelectionForm{Catalogs: record.Home.Catalogs}); err != nil {
			return err
		}
		if err := saveCollectionSelectionTx(ctx, tx, profileID, CollectionSelectionForm{Collections: record.Home.Collections}); err != nil {
			return err
		}
		return WritePushRecord(ctx, tx, profileID, record)
	})
}

// WritePushRecord replaces profileID's push record with record through tx,
// stamped with the Nuvio profile id the profile has now
// (profiles.nuvio_profile_uuid) and the time.
func WritePushRecord(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, record PushRecord) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encoding push record: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO push_records (profile_id, nuvio_profile_uuid, record, pushed_at)
		SELECT id, nuvio_profile_uuid, ?, ? FROM profiles WHERE id = ?
		ON CONFLICT (profile_id) DO UPDATE SET
			nuvio_profile_uuid = excluded.nuvio_profile_uuid, record = excluded.record, pushed_at = excluded.pushed_at
	`, string(raw), utcTimestamp(time.Now()), profileID.String())
	if err != nil {
		return fmt.Errorf("writing push record: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: profile %s not found", ErrInvalidInput, profileID)
	}
	return nil
}

// markNeedsPush marks each of trees against its owner's push record, read
// through q, and returns them.
func markNeedsPush(ctx context.Context, q querier, trees []CollectionWithFolders) ([]CollectionWithFolders, error) {
	pushed, err := pushedCollections(ctx, q, trees)
	if err != nil {
		return nil, err
	}
	for i := range trees {
		trees[i].markNeedsPush(pushed[trees[i].ID])
	}
	return trees, nil
}

// pushedCollections is every collection the push records of trees' owners
// hold, by id, as the bytes push sent for it, read through q. Only an owner
// with a tree on Home is read: off Home, nothing needs a push.
func pushedCollections(ctx context.Context, q querier, trees []CollectionWithFolders) (map[uuid.UUID][]byte, error) {
	var owners []uuid.UUID
	for _, c := range trees {
		if c.HomeSortOrder != nil {
			owners = append(owners, c.OwnerID)
		}
	}
	pushed := make(map[uuid.UUID][]byte)
	if len(owners) == 0 {
		return pushed, nil
	}
	records, err := queryStrings(ctx, q, "push record",
		`SELECT record FROM push_records WHERE profile_id IN (SELECT value FROM json_each(?))`, idsJSON(dedupeUUIDs(owners)))
	if err != nil {
		return nil, err
	}
	for _, raw := range records {
		var record struct {
			Collections []json.RawMessage `json:"collections"`
		}
		if err := json.Unmarshal([]byte(raw), &record); err != nil {
			return nil, fmt.Errorf("decoding push record: %w", err)
		}
		for _, sent := range record.Collections {
			var head struct {
				ID uuid.UUID `json:"id"`
			}
			if err := json.Unmarshal(sent, &head); err != nil {
				return nil, fmt.Errorf("decoding pushed collection: %w", err)
			}
			pushed[head.ID] = sent
		}
	}
	return pushed, nil
}

// markNeedsPush sets tree's NeedsPush when tree is on Home and what push
// would send for it now differs from pushed, what its owner's last push sent
// for it (nil when that push sent nothing for it, which needs a push too). Off
// Home, nothing does.
func (tree *CollectionWithFolders) markNeedsPush(pushed []byte) {
	if tree.HomeSortOrder == nil {
		return
	}
	raw, err := tree.PushJSON()
	tree.NeedsPush = err != nil || !bytes.Equal(raw, pushed)
}
