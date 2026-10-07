// Subscriptions: a copy of someone else's publication that follows it.
// Subscribing writes the snapshot as the caller's own rows, each catalog
// and folder of a collection carrying its snapshot key as sub_key, which
// Update overwrites by key, keeping its ids. Any other content write to the
// copy is refused (refuseSubscribedCopy), so Update only ever meets a copy
// still as it was written; a duplicate is a copy that never subscribed.

package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CommunityCopy is the caller's copy of a publication that a subscribe, a
// duplicate or an Update wrote: a listed catalog or a collection, by the
// publication's kind.
type CommunityCopy struct {
	Kind       string                 `json:"kind"`
	Catalog    *Catalog               `json:"catalog,omitempty"`
	Collection *CollectionWithFolders `json:"collection,omitempty"`
}

// storedPublication is a publications row as a subscribe, a duplicate or an
// Update reads it.
type storedPublication struct {
	id, publisherID        uuid.UUID
	kind, contentHash, raw string
	snapshot               Snapshot
}

// publicationColumns are the columns scanPublication reads, from
// publications as p.
const publicationColumns = `p.id, p.publisher_id, p.kind, p.content_hash, p.snapshot`

// scanPublication reads publicationColumns from row, after dests for any
// columns the caller's query lists ahead of them. A missing row is
// ErrPublicationNotFound.
func scanPublication(row *sql.Row, dests ...any) (storedPublication, error) {
	var pub storedPublication
	var id, publisherID string
	err := row.Scan(append(dests, &id, &publisherID, &pub.kind, &pub.contentHash, &pub.raw)...)
	if errors.Is(err, sql.ErrNoRows) {
		return storedPublication{}, ErrPublicationNotFound
	}
	if err != nil {
		return storedPublication{}, fmt.Errorf("loading publication: %w", err)
	}
	var p rowParser
	pub.id = p.uuid(id, "publication id")
	pub.publisherID = p.uuid(publisherID, "publisher id")
	if p.err != nil {
		return storedPublication{}, p.err
	}
	pub.snapshot, err = decodeSnapshot(pub.raw)
	return pub, err
}

// copyablePublication reads publicationID for a subscribe or a duplicate by
// profileID: it must be someone else's (ErrPublicationNotFound otherwise).
func (db *DB) copyablePublication(ctx context.Context, profileID, publicationID uuid.UUID) (storedPublication, error) {
	return scanPublication(db.conn.QueryRowContext(ctx, `
		SELECT `+publicationColumns+` FROM publications p
		WHERE p.id = ? AND p.publisher_id <> ?
	`, publicationID.String(), profileID.String()))
}

// Subscribe writes publicationID's snapshot as profileID's own rows and
// subscribes them to it: a catalog publication becomes a listed catalog, a
// collection publication a collection with every catalog scoped to it. The
// copy is unpublished, off Home and never pushed. Only the form validators
// run: the snapshot's recipes were checked against TMDB when it was
// published, so a subscribe makes no TMDB call.
// Returns ErrPublicationNotFound unless the publication exists and is
// someone else's, and ErrConflict when profileID already subscribes to it.
func (db *DB) Subscribe(ctx context.Context, profileID, publicationID uuid.UUID) (CommunityCopy, error) {
	return db.copyPublication(ctx, profileID, publicationID, true)
}

// DuplicatePublication is Subscribe without the subscription: a copy of the
// snapshot as profileID's own, fully editable rows, which Community never
// offers an Update for, and any number of which can sit beside a
// subscription. The copy's name or title gets a " (copy)" suffix
// (copyName).
func (db *DB) DuplicatePublication(ctx context.Context, profileID, publicationID uuid.UUID) (CommunityCopy, error) {
	return db.copyPublication(ctx, profileID, publicationID, false)
}

// copyPublication runs a subscribe, or a duplicate when subscribe is false.
func (db *DB) copyPublication(ctx context.Context, profileID, publicationID uuid.UUID, subscribe bool) (CommunityCopy, error) {
	pub, err := db.copyablePublication(ctx, profileID, publicationID)
	if err != nil {
		return CommunityCopy{}, err
	}
	pub.snapshot = pub.snapshot.forCopy(subscribe)
	if err := pub.snapshot.validate(); err != nil {
		return CommunityCopy{}, err
	}
	var copyID uuid.UUID
	err = db.inTx(ctx, func(tx *sql.Tx) error {
		var err error
		if copyID, err = writeCopy(ctx, tx, profileID, pub, subscribe); err != nil || !subscribe {
			return err
		}
		return insertSubscription(ctx, tx, profileID, pub, copyID)
	})
	if err != nil {
		return CommunityCopy{}, err
	}
	return db.loadCopy(ctx, profileID, pub.kind, copyID)
}

// writeCopy writes pub's snapshot as a new row of profileID's, keyed by the
// snapshot's keys when keyed, and returns its id.
func writeCopy(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, pub storedPublication, keyed bool) (uuid.UUID, error) {
	if pub.kind == kindCatalog {
		c, err := insertCatalog(ctx, tx, catalogFromSnapshot(profileID, pub.snapshot))
		return c.ID, err
	}
	created, _, err := createCollectionTx(ctx, tx, profileID, pub.snapshot.collectionForm(keyed))
	return created.ID, err
}

// catalogFromSnapshot is s's catalog as a new listed catalog of profileID's.
func catalogFromSnapshot(profileID uuid.UUID, s Snapshot) Catalog {
	form := s.catalogForm()
	now := time.Now().UTC()
	return Catalog{
		ID: uuid.New(), Type: form.Type, Name: form.Name, Provider: form.Provider, Params: form.Params,
		OwnerID: profileID, CreatedAt: now, UpdatedAt: now,
	}
}

// insertSubscription subscribes copyID, profileID's copy of pub, to it, in
// step with pub's current snapshot, checking in the same statement that pub
// still exists. A second subscription to one publication is ErrConflict,
// and one to a publication unpublished since it was read is
// ErrPublicationNotFound (requireInserted).
func insertSubscription(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, pub storedPublication, copyID uuid.UUID) error {
	catalogID, collectionID := sourceColumns(pub.kind, copyID)
	result, err := tx.ExecContext(ctx, `
		INSERT INTO subscriptions (id, subscriber_id, publication_id, catalog_id, collection_id, subscribed_hash, created_at)
		SELECT ?, ?, ?, ?, ?, ?, ?
		WHERE EXISTS (SELECT 1 FROM publications WHERE id = ?)
	`, uuid.New().String(), profileID.String(), pub.id.String(), catalogID, collectionID, pub.contentHash,
		time.Now().UTC().Format(time.RFC3339), pub.id.String())
	if isUniqueConstraintErr(err) {
		return fmt.Errorf("%w: you already added this", ErrConflict)
	}
	if err != nil {
		return fmt.Errorf("inserting subscription: %w", err)
	}
	return requireInserted(result)
}

// requireInserted is ErrPublicationNotFound when insertSubscription wrote
// no row: the publication was unpublished after the subscribe read it, so the
// transaction rolls the copy back too.
func requireInserted(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return ErrPublicationNotFound
	}
	return nil
}

// loadCopy reads profileID's copy id, of kind, as a CommunityCopy.
func (db *DB) loadCopy(ctx context.Context, profileID uuid.UUID, kind string, id uuid.UUID) (CommunityCopy, error) {
	if kind == kindCatalog {
		c, err := ownCatalog(ctx, db.conn, profileID, id)
		return CommunityCopy{Kind: kind, Catalog: &c}, err
	}
	tree, err := ownCollection(ctx, db.conn, profileID, id)
	return CommunityCopy{Kind: kind, Collection: &tree}, err
}

// subscription is one of the caller's subscriptions, with the publication
// it names.
type subscription struct {
	id, copyID     uuid.UUID
	subscribedHash string
	pub            storedPublication
}

// loadSubscription reads profileID's subscription to publicationID through
// q, or ErrPublicationNotFound when there is none.
func loadSubscription(ctx context.Context, q dbtx, profileID, publicationID uuid.UUID) (subscription, error) {
	var sub subscription
	var id, copyID string
	pub, err := scanPublication(q.QueryRowContext(ctx, `
		SELECT s.id, coalesce(s.catalog_id, s.collection_id), s.subscribed_hash, `+publicationColumns+`
		FROM subscriptions s JOIN publications p ON p.id = s.publication_id
		WHERE s.subscriber_id = ? AND s.publication_id = ?
	`, profileID.String(), publicationID.String()), &id, &copyID, &sub.subscribedHash)
	if err != nil {
		return subscription{}, err
	}
	var p rowParser
	sub.id = p.uuid(id, "subscription id")
	sub.copyID = p.uuid(copyID, "subscribed copy id")
	sub.pub = pub
	return sub, p.err
}

// UpdateSubscription brings profileID's subscribed copy of publicationID
// up to its current snapshot. A catalog copy takes the snapshot's name and
// recipe. A collection copy is overwritten by key through the same update
// core a collection save uses: each of its folders and scoped catalogs
// whose key the snapshot still has keeps its id and takes the snapshot's
// content, the snapshot's other rows are added, and the copy's rows it no
// longer has are removed. The copy's pin and Home placement stay as they
// are, and a copy on Home whose update changes what push sends for it shows
// on Home as needing a push. A copy whose content already equals the snapshot is only
// marked in step; any other is written through the validators a save runs,
// and ErrInvalidInput when they refuse the snapshot. Returns the copy, or
// ErrPublicationNotFound unless profileID subscribes to the publication.
func (db *DB) UpdateSubscription(ctx context.Context, profileID, publicationID uuid.UUID) (CommunityCopy, error) {
	var sub subscription
	err := db.inTx(ctx, func(tx *sql.Tx) error {
		var err error
		if sub, err = loadSubscription(ctx, tx, profileID, publicationID); err != nil {
			return err
		}
		return sub.update(ctx, tx, profileID)
	})
	if err != nil {
		return CommunityCopy{}, err
	}
	return db.loadCopy(ctx, profileID, sub.pub.kind, sub.copyID)
}

// update is UpdateSubscription's write, inside tx.
func (sub subscription) update(ctx context.Context, tx *sql.Tx, profileID uuid.UUID) error {
	if sub.subscribedHash == sub.pub.contentHash {
		return nil
	}
	if err := sub.rewriteCopy(ctx, tx, profileID); err != nil {
		return err
	}
	return sub.markInStep(ctx, tx)
}

// rewriteCopy writes sub's snapshot over its copy; see UpdateSubscription.
func (sub subscription) rewriteCopy(ctx context.Context, tx *sql.Tx, profileID uuid.UUID) error {
	if sub.pub.kind == kindCatalog {
		return updateCatalogCopy(ctx, tx, profileID, sub)
	}
	return updateCollectionCopy(ctx, tx, profileID, sub)
}

// markInStep sets sub's subscribed_hash to its publication's content hash.
func (sub subscription) markInStep(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `UPDATE subscriptions SET subscribed_hash = ? WHERE id = ?`, sub.pub.contentHash, sub.id.String()); err != nil {
		return fmt.Errorf("updating subscription: %w", err)
	}
	return nil
}

// updateCatalogCopy writes sub's snapshot catalog over its catalog copy,
// unless the copy already holds it; see catalogCopyProblem for what is
// refused.
func updateCatalogCopy(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, sub subscription) error {
	c, err := ownCatalog(ctx, tx, profileID, sub.copyID)
	if err != nil {
		return err
	}
	want := sub.pub.snapshot.catalogForm()
	if copySnapshot(c, sub.pub.snapshot).contentHash() == sub.pub.contentHash {
		return nil
	}
	if err := catalogCopyProblem(c, want); err != nil {
		return err
	}
	return writeCatalogCopy(ctx, tx, c.ID, want)
}

// catalogCopyProblem is why want can't be written over catalog copy c: a
// snapshot of another type, since a catalog's type never changes, or one a
// catalog save would refuse.
func catalogCopyProblem(c Catalog, want CatalogForm) error {
	if !sameKind(catalogKind{c.Type, c.Provider}, catalogKind{want.Type, want.Provider}) {
		return fmt.Errorf("%w: its publisher changed its type, so this update can't apply to your copy; duplicate it from Community instead", ErrInvalidInput)
	}
	return want.Validate()
}

// writeCatalogCopy writes want's name and params over catalog id, whose type
// and provider want already matches (catalogCopyProblem).
func writeCatalogCopy(ctx context.Context, tx *sql.Tx, id uuid.UUID, want CatalogForm) error {
	nowStr := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE catalogs SET name = ?, params = ?, updated_at = ? WHERE id = ?`,
		want.Name, want.Params, nowStr, id.String()); err != nil {
		return fmt.Errorf("updating subscribed catalog: %w", err)
	}
	return nil
}

// copySnapshot is catalog copy c as a snapshot under the key of its
// publication's catalog, which s holds, so the two compare.
func copySnapshot(c Catalog, s Snapshot) Snapshot {
	return catalogSnapshotKeyed(s.Catalogs[0].Key, c)
}

// updateCollectionCopy writes sub's snapshot over its collection copy by
// key, unless the copy already holds it; see UpdateSubscription.
func updateCollectionCopy(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, sub subscription) error {
	tree, err := ownCollection(ctx, tx, profileID, sub.copyID)
	if err != nil {
		return err
	}
	if copyTreeSnapshot(tree).contentHash() == sub.pub.contentHash {
		return nil
	}
	form := updateFormFromSnapshot(sub.pub.snapshot, tree)
	if err := form.Validate(); err != nil {
		return err
	}
	return updateCollectionTx(ctx, tx, profileID, tree.ID, form, time.Now().UTC().Format(time.RFC3339))
}

// copyTreeSnapshot is a subscribed collection copy as a snapshot under its
// own sub_keys, so it compares with its publication's: a row without one
// takes a key no snapshot has.
func copyTreeSnapshot(tree CollectionWithFolders) Snapshot {
	return keyedSnapshot(tree,
		func(c Catalog) string { return subKeyOr(c.SubKey, c.ID) },
		func(f Folder) string { return subKeyOr(f.SubKey, f.ID) })
}

// subKeyOr is subKey, or a key naming row id when subKey is empty.
func subKeyOr(subKey string, id uuid.UUID) string {
	if subKey == "" {
		return "copy:" + id.String()
	}
	return subKey
}

// updateFormFromSnapshot builds the CollectionForm that writes s over
// subscribed copy tree by key; see UpdateSubscription. A snapshot catalog
// whose key names one of the copy's scoped catalogs of the same type and
// provider is a catalog edit of it, and its refs point at it; any other is
// a New entry. A snapshot folder whose key names one of the copy's folders
// keeps that folder's id.
func updateFormFromSnapshot(s Snapshot, tree CollectionWithFolders) CollectionForm {
	m := matchCopyCatalogs(s.Catalogs, scopedCatalogsByKey(tree))
	bc := s.bundleCollection()
	bc.Catalogs = m.fresh
	form := collectionFormFromBundle(bc, m.counterparts, true)
	form.CatalogEdits = m.edits
	folders := foldersByKey(tree)
	for i, f := range s.Collection.Folders {
		form.Folders[i].SubKey = f.Key
		if id, ok := folders[f.Key]; ok {
			form.Folders[i].ID = &id
		}
	}
	return form
}

// catalogMatch is how a snapshot's catalogs pair with a copy's: the copy's
// catalog each matched key points at, the edit that writes the snapshot's
// catalog over it, and the snapshot's catalogs that match none.
type catalogMatch struct {
	counterparts map[string]uuid.UUID
	edits        []ScopedCatalogEdit
	fresh        []BundleCatalog
}

// matchCopyCatalogs pairs each of catalogs with the copy catalog copies holds
// under its key, when that one has the same type and provider.
func matchCopyCatalogs(catalogs []BundleCatalog, copies map[string]Catalog) catalogMatch {
	m := catalogMatch{counterparts: map[string]uuid.UUID{}, edits: []ScopedCatalogEdit{}, fresh: []BundleCatalog{}}
	for _, c := range catalogs {
		counterpart, ok := copies[c.Key]
		if !ok || !sameKind(catalogKind{counterpart.Type, counterpart.Provider}, catalogKind{c.Type, c.Provider}) {
			m.fresh = append(m.fresh, c)
			continue
		}
		m.counterparts[c.Key] = counterpart.ID
		m.edits = append(m.edits, ScopedCatalogEdit{ID: counterpart.ID, Type: c.Type, Provider: c.Provider, Name: c.Name, Params: string(c.Params)})
	}
	return m
}

// scopedCatalogsByKey maps each sub_key of a catalog scoped to tree to that
// catalog.
func scopedCatalogsByKey(tree CollectionWithFolders) map[string]Catalog {
	byKey := map[string]Catalog{}
	for _, c := range tree.Catalogs {
		if c.SubKey != "" && c.CollectionID != nil {
			byKey[c.SubKey] = c
		}
	}
	return byKey
}

// foldersByKey maps each sub_key of tree's folders to that folder's id.
func foldersByKey(tree CollectionWithFolders) map[string]uuid.UUID {
	byKey := map[string]uuid.UUID{}
	for _, f := range tree.Folders {
		if f.SubKey != "" {
			byKey[f.SubKey] = f.ID
		}
	}
	return byKey
}
