// The push record: what a profile's last push put in Nuvio, kept whole in
// push_records — each collection as the bytes sent, the Home selection, and
// every catalog Nuvio can reach with its name, type and params inline. One
// builder makes it (buildPushRecord): push builds it before contacting Nuvio,
// sends its collections, and stores it once Nuvio has accepted them.

package vault

import (
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

// catalogIDs is the id of every catalog in h, in order.
func (h PushedHome) catalogIDs() []uuid.UUID {
	ids := make([]uuid.UUID, len(h.Catalogs))
	for i, c := range h.Catalogs {
		ids[i] = c.CatalogID
	}
	return ids
}

// collectionIDs is the id of every collection in h, in order.
func (h PushedHome) collectionIDs() []uuid.UUID {
	ids := make([]uuid.UUID, len(h.Collections))
	for i, c := range h.Collections {
		ids[i] = c.CollectionID
	}
	return ids
}

// PushedCatalog is one catalog Nuvio can reach, as a push left it.
type PushedCatalog struct {
	ID       uuid.UUID       `json:"id"`
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Provider string          `json:"provider"`
	Params   json.RawMessage `json:"params"`
}

// BuildPushRecord is what a push of home, profileID's pending Home selection,
// puts in Nuvio now. Push calls it before contacting Nuvio, so an id the
// selection may not hold — not profileID's own, or a catalog that isn't
// listed — is refused here (inSelectionOrder), not by the local write after
// Nuvio took the push. Every row is read in one snapshot, so a save landing
// meanwhile can't leave a folder naming a catalog the record lacks.
func (db *DB) BuildPushRecord(ctx context.Context, profileID uuid.UUID, home PushedHome) (record PushRecord, err error) {
	err = db.inReadTx(ctx, func(q dbtx) (err error) {
		record, err = buildPushRecord(ctx, q, profileID, home)
		return err
	})
	return record, err
}

// StoredPushRecord is what a push of profileID's Home as push last stored it
// (the Home columns) puts in Nuvio now, read through q: what the
// waiting-for-push list compares the held record with.
func StoredPushRecord(ctx context.Context, q dbtx, profileID uuid.UUID) (PushRecord, error) {
	listed, err := selectLeanCatalogs(ctx, q, "c.owner_id = ? AND c.home_sort_order IS NOT NULL", profileID.String())
	if err != nil {
		return PushRecord{}, err
	}
	slices.SortFunc(listed, compareByHomeSortOrder)
	var home PushedHome
	for _, c := range listed {
		home.Catalogs = append(home.Catalogs, SelectedCatalogInput{CatalogID: c.ID, ShowInHome: c.ShowInHome, Position: *c.HomeSortOrder})
	}

	onHome, err := selectLeanCollections(ctx, q, "col.owner_id = ? AND col.home_sort_order IS NOT NULL", profileID.String())
	if err != nil {
		return PushRecord{}, err
	}
	slices.SortFunc(onHome, compareCollectionsByHomeSortOrder)
	for _, c := range onHome {
		home.Collections = append(home.Collections, SelectedCollectionInput{CollectionID: c.ID, PinToTop: c.PinToTop, Position: *c.HomeSortOrder})
	}
	return buildPushRecord(ctx, q, profileID, home)
}

// buildPushRecord is the push record of home, profileID's Home selection, read
// through q.
func buildPushRecord(ctx context.Context, q dbtx, profileID uuid.UUID, home PushedHome) (PushRecord, error) {
	trees, err := selectedTrees(ctx, q, profileID, home)
	if err != nil {
		return PushRecord{}, err
	}
	listed, err := selectedCatalogs(ctx, q, profileID, home)
	if err != nil {
		return PushRecord{}, err
	}

	record := PushRecord{
		Collections: make([]json.RawMessage, 0, len(trees)),
		Home: PushedHome{
			Catalogs:    jsonwire.OrEmpty(home.Catalogs),
			Collections: jsonwire.OrEmpty(home.Collections),
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

// selectedTrees is profileID's collections home names, each with its tree,
// in Home order and with the pin its entry carries, read through q. One that
// isn't profileID's own is ErrInvalidInput.
func selectedTrees(ctx context.Context, q dbtx, profileID uuid.UUID, home PushedHome) ([]CollectionWithFolders, error) {
	ids := home.collectionIDs()
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
	return applySelection(trees, home)
}

// applySelection is trees in home's order, each with the pin its entry
// carries: the pin a push sends is the pending one, which only the write after
// Nuvio accepted the push stores. An entry trees lacks is ErrInvalidInput
// (inSelectionOrder).
func applySelection(trees []CollectionWithFolders, home PushedHome) ([]CollectionWithFolders, error) {
	ordered, err := inSelectionOrder("collection", home.collectionIDs(), trees, treeID)
	if err != nil {
		return nil, err
	}
	for i, entry := range home.Collections {
		ordered[i].PinToTop = entry.PinToTop
	}
	return ordered, nil
}

// selectedCatalogs is profileID's listed catalogs home names, in Home order,
// read through q. One that isn't profileID's own, or is scoped to a
// collection, is ErrInvalidInput.
func selectedCatalogs(ctx context.Context, q dbtx, profileID uuid.UUID, home PushedHome) ([]Catalog, error) {
	ids := home.catalogIDs()
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := selectLeanCatalogs(ctx, q, "c.id IN (SELECT value FROM json_each(?)) AND c.owner_id = ? AND c.collection_id IS NULL",
		idsJSON(ids), profileID.String())
	if err != nil {
		return nil, err
	}
	return inSelectionOrder("catalog", ids, rows, catalogID)
}

// inSelectionOrder is read, the rows a build read for ids, in the order of
// ids, a repeated id repeating its row. An id read lacks is one the Home
// selection may not hold, and is ErrInvalidInput naming it: refused here,
// before push contacts Nuvio, rather than by SavePush's write once Nuvio has
// taken the push.
func inSelectionOrder[T any](label string, ids []uuid.UUID, read []T, idOf func(T) uuid.UUID) ([]T, error) {
	index := indexByID(read, idOf)
	ordered := make([]T, len(ids))
	for i, id := range ids {
		j, ok := index[id]
		if !ok {
			return nil, fmt.Errorf("%w: %s %s is not accessible to this profile", ErrInvalidInput, label, id)
		}
		ordered[i] = read[j]
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
// save landing during the push still shows as waiting for the next one. It
// raises profileID's home_revision and returns the new one (writePushedHome).
func (db *DB) SavePush(ctx context.Context, profileID uuid.UUID, record PushRecord) (revision int64, err error) {
	err = db.inTx(ctx, func(tx *sql.Tx) (err error) {
		if err := saveCatalogSelectionTx(ctx, tx, profileID, record.Home.Catalogs); err != nil {
			return err
		}
		if err := saveCollectionSelectionTx(ctx, tx, profileID, record.Home.Collections); err != nil {
			return err
		}
		revision, err = writePushedHome(ctx, tx, profileID, record)
		return err
	})
	return revision, err
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

// currentRecords is the push records that still describe their profile's Nuvio
// profile: the record's stamp is the profile's Nuvio profile id now. A record
// stamped with another id was pushed to a Nuvio profile the slot has since
// lost, so Nuvio holds none of it. It names push_records pr and profiles p.
const currentRecords = `push_records pr JOIN profiles p
	ON p.id = pr.profile_id AND p.nuvio_profile_uuid = pr.nuvio_profile_uuid`

// heldRecord is what profileID's Nuvio profile holds: the push record, read
// through q, when it is current (currentRecords). ok is false when there is
// none, which is what Nuvio holds for a profile that never pushed or whose
// slot was reused since.
func heldRecord(ctx context.Context, q dbtx, profileID uuid.UUID) (record PushRecord, ok bool, err error) {
	raws, err := queryStrings(ctx, q, "push record",
		`SELECT pr.record FROM `+currentRecords+` WHERE pr.profile_id = ?`, profileID.String())
	if err != nil || len(raws) == 0 {
		return PushRecord{}, false, err
	}
	if err := json.Unmarshal([]byte(raws[0]), &record); err != nil {
		return PushRecord{}, false, fmt.Errorf("decoding push record: %w", err)
	}
	return record, true, nil
}

// PushedCollectionIDs is the id of every collection profileID's last push sent
// Nuvio, none when Nuvio holds none of it (heldRecord). Push drops these from
// Nuvio's blob along with the collections the profile owns, so one deleted
// since is dropped too, however few sources it had.
func (db *DB) PushedCollectionIDs(ctx context.Context, profileID uuid.UUID) ([]uuid.UUID, error) {
	record, _, err := heldRecord(ctx, db.conn, profileID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(record.Collections))
	for _, raw := range record.Collections {
		head, err := pushedHeadOf(raw)
		if err != nil {
			return nil, err
		}
		ids = append(ids, head.ID)
	}
	return ids, nil
}

// pushedHead is the id and title of one collection a push sent, as its bytes
// carry them.
type pushedHead struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
}

func pushedHeadOf(raw json.RawMessage) (pushedHead, error) {
	var head pushedHead
	if err := json.Unmarshal(raw, &head); err != nil {
		return pushedHead{}, fmt.Errorf("decoding pushed collection: %w", err)
	}
	return head, nil
}

// selectedCatalogs is every catalog the record holds as the addon's manifest
// lists it, in the record's order: those with a Home row of their own with
// the Home or Discover the record carries in ShowInHome, then the ones only a
// folder uses, all off Home.
func (r PushRecord) selectedCatalogs() []Catalog {
	showInHome := make(map[uuid.UUID]bool, len(r.Home.Catalogs))
	for _, c := range r.Home.Catalogs {
		showInHome[c.CatalogID] = c.ShowInHome
	}
	out := make([]Catalog, len(r.Catalogs))
	for i, c := range r.Catalogs {
		out[i] = Catalog{
			ID: c.ID, Type: c.Type, Name: c.Name, Provider: c.Provider, Params: string(c.Params),
			ShowInHome: showInHome[c.ID],
		}
	}
	return out
}

// HomeRow is one row a push puts on Nuvio's home screen: a collection by
// its id, or a catalog by its type and the id the addon's manifest lists it
// by (ManifestID).
type HomeRow struct {
	CollectionID uuid.UUID
	Type         string
	CatalogID    string
	// position is the row's place on Home, which HomeRows orders by.
	position int
}

// HomeRows is r's home-screen rows in the order Nuvio shows them: pinned
// holds the pinned collections, and rows the catalogs with a home row of
// their own and the other collections together, each list in Position
// order. A catalog in Discover only, or one only a folder uses, has no row.
func (r PushRecord) HomeRows() (pinned, rows []HomeRow) {
	pinned = r.collectionRows(true)
	rows = append(r.catalogRows(), r.collectionRows(false)...)
	slices.SortStableFunc(pinned, byPosition)
	slices.SortStableFunc(rows, byPosition)
	return pinned, rows
}

// byPosition orders two home rows by their place on Home.
func byPosition(a, b HomeRow) int { return a.position - b.position }

// catalogRows is r's catalogs with a home row of their own.
func (r PushRecord) catalogRows() []HomeRow {
	pushed := r.pushedCatalogs()
	var rows []HomeRow
	for _, c := range r.Home.Catalogs {
		if c.ShowInHome {
			pc := pushed[c.CatalogID]
			rows = append(rows, HomeRow{Type: pc.Type, CatalogID: ManifestID(Catalog{ID: pc.ID, Provider: pc.Provider}), position: c.Position})
		}
	}
	return rows
}

// pushedCatalogs is r's catalogs by id.
func (r PushRecord) pushedCatalogs() map[uuid.UUID]PushedCatalog {
	pushed := make(map[uuid.UUID]PushedCatalog, len(r.Catalogs))
	for _, c := range r.Catalogs {
		pushed[c.ID] = c
	}
	return pushed
}

// collectionRows is r's collections pinned or not, as pinned says.
func (r PushRecord) collectionRows(pinned bool) []HomeRow {
	var rows []HomeRow
	for _, c := range r.Home.Collections {
		if c.PinToTop == pinned {
			rows = append(rows, HomeRow{CollectionID: c.CollectionID, position: c.Position})
		}
	}
	return rows
}
