package migrations

import (
	"cmp"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
)

// publications replaces sharing by a public flag and linked copies with
// publications and subscriptions:
//
//  1. Every public catalog or collection that is not a linked copy becomes a
//     live publication of its content as it stands: a snapshot in the
//     publication format, taken with no validation and no network, exactly
//     what Community showed of it. A collection that uses a catalog step 2
//     makes a subscription stays unpublished, since only that catalog's
//     publisher can share it.
//  2. Every linked copy whose source became a publication becomes a
//     subscription to it. The subscription is in step, its taken_hash the
//     publication's content hash, when the source still hashes to the copy's
//     taken_hash or the copy already equals its source; otherwise it reads as
//     out of step. A copied collection's
//     scoped catalogs get their sub_key from the catalog they were taken
//     from, and its folders theirs by position, as Update paired them.
//  3. Every other link, to a private source, a source that is itself a copy
//     or a public source left unpublished, is dropped: the copy keeps its
//     content and gets no updates.
//  4. catalogs and collections are rebuilt without is_public, taken_from and
//     taken_hash (and the legacy is_default, where a database still has it),
//     with every row, id and version kept, and catalogs.recipe_hash made NOT
//     NULL. Their indexes and triggers are recreated, and the publications
//     and subscriptions tables, the sub_key columns and their triggers are
//     added.
//
// Publication and subscription ids are derived from the rows they come from,
// so the same database always migrates to the same result. Everything here
// is frozen: the snapshot format, the stable keys and the link hashes are
// this migration's own copies.
func publications(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := loadShareRows(ctx, tx)
	if err != nil {
		return nil, err
	}
	plan, err := planSharing(ctx, tx, rows)
	if err != nil {
		return nil, err
	}
	legacy, err := rebuildForSharing(ctx, tx)
	if err != nil {
		return nil, err
	}
	return plan.notes(legacy), plan.write(ctx, tx)
}

// rebuildForSharing runs publicationsDDL, and reports whether the tables it
// rebuilt still had the legacy is_default column, which the rebuild drops.
func rebuildForSharing(ctx context.Context, tx *sql.Tx) (bool, error) {
	legacy, err := hasLegacyDefault(ctx, tx)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, publicationsDDL); err != nil {
		return false, fmt.Errorf("rebuilding catalogs and collections: %w", err)
	}
	return legacy, nil
}

// shareCatalog is one catalogs row as migration 3 reads it, with its recipe.
type shareCatalog struct {
	id, ownerID, name, catalogType, provider, params, recipeHash, createdAt, updatedAt string
	collectionID, takenFrom, takenHash                                                 sql.NullString
	public                                                                             bool
}

// shareCollection is one collections row as migration 3 reads it.
type shareCollection struct {
	id, ownerID, title, viewMode, backdropImageURL, createdAt, updatedAt string
	showAllTab, focusGlowEnabled, public                                 bool
	takenFrom, takenHash                                                 sql.NullString
}

// shareRows is every catalog and collection, by id, and each kind's ids in
// order.
type shareRows struct {
	catalogs       map[string]shareCatalog
	collections    map[string]shareCollection
	catalogIDs     []string
	collectionIDs  []string
	catalogsByColl map[string][]string
}

// loadShareRows reads every catalog, with its recipe, and every collection.
func loadShareRows(ctx context.Context, tx *sql.Tx) (shareRows, error) {
	r := shareRows{catalogs: map[string]shareCatalog{}, collections: map[string]shareCollection{}, catalogsByColl: map[string][]string{}}
	if err := r.loadCatalogs(ctx, tx); err != nil {
		return r, err
	}
	return r, r.loadCollections(ctx, tx)
}

// loadCatalogs reads every catalog into r, in id order.
func (r *shareRows) loadCatalogs(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT c.id, c.owner_id, c.name, r.type, r.provider, r.params, c.recipe_hash, c.created_at, c.updated_at,
		       c.collection_id, c.taken_from, c.taken_hash, c.is_public <> 0
		FROM catalogs c JOIN recipes r ON r.hash = c.recipe_hash ORDER BY c.id`)
	if err != nil {
		return fmt.Errorf("reading catalogs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var c shareCatalog
		if err := rows.Scan(&c.id, &c.ownerID, &c.name, &c.catalogType, &c.provider, &c.params, &c.recipeHash, &c.createdAt, &c.updatedAt,
			&c.collectionID, &c.takenFrom, &c.takenHash, &c.public); err != nil {
			return fmt.Errorf("reading catalogs: %w", err)
		}
		r.catalogs[c.id] = c
		r.catalogIDs = append(r.catalogIDs, c.id)
		if c.collectionID.Valid {
			r.catalogsByColl[c.collectionID.String] = append(r.catalogsByColl[c.collectionID.String], c.id)
		}
	}
	return rows.Err()
}

// loadCollections reads every collection into r, in id order.
func (r *shareRows) loadCollections(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, owner_id, title, view_mode, backdrop_image_url, created_at, updated_at,
		       show_all_tab <> 0, focus_glow_enabled <> 0, is_public <> 0, taken_from, taken_hash
		FROM collections ORDER BY id`)
	if err != nil {
		return fmt.Errorf("reading collections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var c shareCollection
		if err := rows.Scan(&c.id, &c.ownerID, &c.title, &c.viewMode, &c.backdropImageURL, &c.createdAt, &c.updatedAt,
			&c.showAllTab, &c.focusGlowEnabled, &c.public, &c.takenFrom, &c.takenHash); err != nil {
			return fmt.Errorf("reading collections: %w", err)
		}
		r.collections[c.id] = c
		r.collectionIDs = append(r.collectionIDs, c.id)
	}
	return rows.Err()
}

// shareFolder is one folder of a tree: its id, its content, and its refs by
// catalog id, in order.
type shareFolder struct {
	id   string
	body pubFolderBody
	refs []shareRef
}

// shareRef is one folder ref: the catalog it names and its genre.
type shareRef struct{ catalogID, genre string }

// loadShareFolders reads collection id's folders in order, each with its
// refs in order. A ref to a catalog that isn't there is dropped.
func loadShareFolders(ctx context.Context, tx *sql.Tx, id string) ([]shareFolder, error) {
	folders, err := readShareFolders(ctx, tx, id)
	if err != nil {
		return nil, fmt.Errorf("reading collection %s's folders: %w", id, err)
	}
	for i := range folders {
		if folders[i].refs, err = loadShareRefs(ctx, tx, folders[i].id); err != nil {
			return nil, err
		}
	}
	return folders, nil
}

// readShareFolders reads collection id's folders in order, without their
// refs.
func readShareFolders(ctx context.Context, tx *sql.Tx, id string) ([]shareFolder, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, title, tile_shape, hide_title <> 0, cover_emoji, cover_image_url, focus_gif_url, focus_gif_enabled <> 0,
		       hero_backdrop_url, hero_video_url, title_logo_url
		FROM folders WHERE collection_id = ? ORDER BY sort_order`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	folders := []shareFolder{}
	for rows.Next() {
		var f shareFolder
		b := &f.body
		if err := rows.Scan(&f.id, &b.Title, &b.TileShape, &b.HideTitle, &b.CoverEmoji, &b.CoverImageURL, &b.FocusGIFURL, &b.FocusGIFEnabled,
			&b.HeroBackdropURL, &b.HeroVideoURL, &b.TitleLogoURL); err != nil {
			return nil, err
		}
		folders = append(folders, f)
	}
	return folders, rows.Err()
}

// loadShareRefs reads folder id's refs in order. A ref whose catalog is gone
// is dropped.
func loadShareRefs(ctx context.Context, tx *sql.Tx, folderID string) ([]shareRef, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT fc.catalog_id, fc.genre FROM folder_catalogs fc JOIN catalogs c ON c.id = fc.catalog_id
		WHERE fc.folder_id = ? ORDER BY fc.sort_order`, folderID)
	if err != nil {
		return nil, fmt.Errorf("reading folder %s's refs: %w", folderID, err)
	}
	defer func() { _ = rows.Close() }()
	refs := []shareRef{}
	for rows.Next() {
		var ref shareRef
		if err := rows.Scan(&ref.catalogID, &ref.genre); err != nil {
			return nil, fmt.Errorf("reading folder %s's refs: %w", folderID, err)
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

// referencedCatalogs is the id of every catalog folders reference, in the
// order the refs first name each.
func referencedCatalogs(folders []shareFolder) []string {
	seen := map[string]bool{}
	var ids []string
	for _, f := range folders {
		for _, ref := range f.refs {
			if !seen[ref.catalogID] {
				seen[ref.catalogID] = true
				ids = append(ids, ref.catalogID)
			}
		}
	}
	return ids
}

// keyedBodies is each of folders' content with every ref naming its catalog
// by key(catalog id).
func keyedBodies(folders []shareFolder, key func(string) string) []pubFolderBody {
	bodies := make([]pubFolderBody, len(folders))
	for i, f := range folders {
		body := f.body
		body.Refs = make([]pubRef, len(f.refs))
		for j, ref := range f.refs {
			body.Refs[j] = pubRef{Catalog: key(ref.catalogID), Genre: ref.genre}
		}
		bodies[i] = body
	}
	return bodies
}

// pubSnapshot, pubCatalog, pubCollection, pubFolder, pubFolderBody and pubRef
// are the publication snapshot format, version 1: every catalog the
// publication holds with its params inline, under its stable key, and for a
// collection its own fields and its folders, each under its stable key, with
// every ref naming a catalog by key.
type pubSnapshot struct {
	Format     string         `json:"format"`
	Version    int            `json:"version"`
	Catalogs   []pubCatalog   `json:"catalogs"`
	Collection *pubCollection `json:"collection,omitempty"`
}

type pubCatalog struct {
	Key      string          `json:"key"`
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Provider string          `json:"provider"`
	Params   json.RawMessage `json:"params"`
}

type pubCollection struct {
	Title            string      `json:"title"`
	ViewMode         string      `json:"view_mode"`
	ShowAllTab       bool        `json:"show_all_tab"`
	BackdropImageURL string      `json:"backdrop_image_url"`
	FocusGlowEnabled bool        `json:"focus_glow_enabled"`
	Folders          []pubFolder `json:"folders"`
}

type pubFolder struct {
	Key string `json:"key"`
	pubFolderBody
}

type pubFolderBody struct {
	Title           string   `json:"title"`
	TileShape       string   `json:"tile_shape"`
	HideTitle       bool     `json:"hide_title"`
	CoverEmoji      string   `json:"cover_emoji"`
	CoverImageURL   string   `json:"cover_image_url"`
	FocusGIFURL     string   `json:"focus_gif_url"`
	FocusGIFEnabled bool     `json:"focus_gif_enabled"`
	HeroBackdropURL string   `json:"hero_backdrop_url"`
	HeroVideoURL    string   `json:"hero_video_url"`
	TitleLogoURL    string   `json:"title_logo_url"`
	Refs            []pubRef `json:"refs"`
}

type pubRef struct {
	Catalog string `json:"catalog"`
	Genre   string `json:"genre"`
}

// The snapshot format this migration writes.
const (
	snapshotFormat  = "uno-publication"
	snapshotVersion = 1
)

// stableKey is the key a publication's snapshot gives the catalog or folder
// sourceID: the first 16 hex digits of the sha256 of the publication id and
// the source row id, so it stays the same across republishes.
func stableKey(publicationID, sourceID string) string {
	return shaHex([]byte(publicationID + sourceID))[:16]
}

// shaHex is the hex sha256 of b.
func shaHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// derivedID is a UUID, version 8, made from the sha256 of seed: the id
// migration 3 gives a row it creates from another, so a database always
// migrates to the same ids.
func derivedID(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	sum[6] = sum[6]&0x0f | 0x80
	sum[8] = sum[8]&0x3f | 0x80
	h := hex.EncodeToString(sum[:16])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// snapshotCatalog is c under key, its params inline.
func snapshotCatalog(key string, c shareCatalog) pubCatalog {
	return pubCatalog{Key: key, Name: c.name, Type: c.catalogType, Provider: c.provider, Params: json.RawMessage(c.params)}
}

// catalogSnapshot is the snapshot of listed catalog c published as
// publicationID.
func catalogSnapshot(publicationID string, c shareCatalog) pubSnapshot {
	return pubSnapshot{
		Format: snapshotFormat, Version: snapshotVersion,
		Catalogs: []pubCatalog{snapshotCatalog(stableKey(publicationID, c.id), c)},
	}
}

// collectionSnapshot is the snapshot of collection c, with folders, published
// as publicationID: every catalog its folders reference, in the order they
// are first referenced.
func collectionSnapshot(publicationID string, c shareCollection, folders []shareFolder, catalogs map[string]shareCatalog) pubSnapshot {
	key := func(id string) string { return stableKey(publicationID, id) }
	ids := referencedCatalogs(folders)
	snapshotCatalogs := make([]pubCatalog, len(ids))
	for i, id := range ids {
		snapshotCatalogs[i] = snapshotCatalog(key(id), catalogs[id])
	}
	bodies := keyedBodies(folders, key)
	snapshotFolders := make([]pubFolder, len(folders))
	for i, f := range folders {
		snapshotFolders[i] = pubFolder{Key: key(f.id), pubFolderBody: bodies[i]}
	}
	return pubSnapshot{
		Format: snapshotFormat, Version: snapshotVersion, Catalogs: snapshotCatalogs,
		Collection: &pubCollection{
			Title: c.title, ViewMode: c.viewMode, ShowAllTab: c.showAllTab,
			BackdropImageURL: c.backdropImageURL, FocusGlowEnabled: c.focusGlowEnabled, Folders: snapshotFolders,
		},
	}
}

// folderCount is how many folders s holds.
func (s pubSnapshot) folderCount() int {
	if s.Collection == nil {
		return 0
	}
	return len(s.Collection.Folders)
}

// catalogLinkHashV2 is a listed catalog's link hash as migration 2 left
// taken_hash: sha256 hex of its length-prefixed name and its recipe hash.
func catalogLinkHashV2(c shareCatalog) string {
	return shaHex([]byte(strconv.Itoa(len(c.name)) + ":" + c.name + c.recipeHash))
}

// linkCollection is the form a collection's link hash, as migration 2 left
// taken_hash, is taken over: its own fields, every catalog it references as
// c1, c2, … in the order its refs first name each, with the recipe hash in
// place of params, and its folders.
type linkCollection struct {
	Title            string          `json:"title"`
	ViewMode         string          `json:"view_mode"`
	ShowAllTab       bool            `json:"show_all_tab"`
	BackdropImageURL string          `json:"backdrop_image_url"`
	FocusGlowEnabled bool            `json:"focus_glow_enabled"`
	Catalogs         []pubCatalog    `json:"catalogs"`
	Folders          []pubFolderBody `json:"folders"`
}

// collectionLinkHashV2 is collection c's link hash, with folders, as
// migration 2 left taken_hash.
func collectionLinkHashV2(c shareCollection, folders []shareFolder, catalogs map[string]shareCatalog) (string, error) {
	ids := referencedCatalogs(folders)
	keys := make(map[string]string, len(ids))
	linkCatalogs := make([]pubCatalog, len(ids))
	for i, id := range ids {
		keys[id] = "c" + strconv.Itoa(i+1)
		recipeHash, err := json.Marshal(catalogs[id].recipeHash)
		if err != nil {
			return "", err
		}
		linkCatalogs[i] = pubCatalog{Key: keys[id], Name: catalogs[id].name, Type: catalogs[id].catalogType, Provider: catalogs[id].provider, Params: recipeHash}
	}
	b, err := json.Marshal(linkCollection{
		Title: c.title, ViewMode: c.viewMode, ShowAllTab: c.showAllTab, BackdropImageURL: c.backdropImageURL,
		FocusGlowEnabled: c.focusGlowEnabled, Catalogs: linkCatalogs,
		Folders: keyedBodies(folders, func(id string) string { return keys[id] }),
	})
	if err != nil {
		return "", fmt.Errorf("hashing collection %s: %w", c.id, err)
	}
	return shaHex(b), nil
}

// pubRow is one publications row migration 3 writes.
type pubRow struct {
	id, ownerID, kind, sourceID, title, snapshot, contentHash, publishedAt, updatedAt string
	catalogCount, folderCount                                                         int
}

// newPubRow is the live publication, id publicationID, of source, holding s.
func newPubRow(publicationID, kind string, s pubSnapshot, source shareSource) (pubRow, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return pubRow{}, fmt.Errorf("writing the snapshot of %s %s: %w", kind, source.id, err)
	}
	return pubRow{
		id: publicationID, ownerID: source.ownerID, kind: kind, sourceID: source.id, title: source.title,
		snapshot: string(b), contentHash: shaHex(b),
		publishedAt: source.createdAt, updatedAt: source.updatedAt,
		catalogCount: len(s.Catalogs), folderCount: s.folderCount(),
	}, nil
}

// shareSource is what a publication records of the row it publishes.
type shareSource struct{ id, ownerID, title, createdAt, updatedAt string }

// subRow is one subscriptions row migration 3 writes.
type subRow struct {
	id, ownerID, publicationID, kind, copyID, takenHash, createdAt string
}

// outOfStep is the taken_hash of a subscription migrated from a copy that
// was not in step with its source: it matches no content hash, so the
// subscription reads as having an update available.
const outOfStep = "out of step at migration 3"

// linkCounts is how one kind of link fared: subscriptions in and out of
// step, the in-step ones only because the copy already equals its source,
// and links dropped for a private source, a source that is itself a copy, or
// a public source left unpublished.
type linkCounts struct {
	inStep, outOfStep, identical, private, copyOfCopy, unpublished int
}

// sharePlan is everything migration 3 writes after the rebuild, and the
// counts its notes report.
type sharePlan struct {
	publications                            []pubRow
	subscriptions                           []subRow
	catalogKeys, folderKeys                 map[string]string
	catalogLinks, collectionLinks           linkCounts
	publishedCatalogs, publishedCollections int
	reshared, withheld                      int
}

// planSharing works out the publications, subscriptions and sub_keys rows
// make; see publications.
func planSharing(ctx context.Context, tx *sql.Tx, rows shareRows) (sharePlan, error) {
	p := sharePlan{catalogKeys: map[string]string{}, folderKeys: map[string]string{}}
	catalogPubs, err := p.publishCatalogs(rows)
	if err != nil {
		return p, err
	}
	p.subscribeCatalogs(rows, catalogPubs)
	collectionPubs, err := p.publishCollections(ctx, tx, rows)
	if err != nil {
		return p, err
	}
	if err := p.subscribeCollections(ctx, tx, rows, collectionPubs); err != nil {
		return p, err
	}
	slices.SortFunc(p.publications, func(a, b pubRow) int { return cmp.Compare(a.id, b.id) })
	slices.SortFunc(p.subscriptions, func(a, b subRow) int { return cmp.Compare(a.id, b.id) })
	return p, nil
}

// isOriginal reports whether a row public and taken from takenFrom is
// published: public, and not a linked copy.
func isOriginal(public bool, takenFrom sql.NullString) bool {
	return public && !takenFrom.Valid
}

// publishCatalogs adds a publication for every public listed catalog that
// isn't a linked copy, and returns them by source id.
func (p *sharePlan) publishCatalogs(rows shareRows) (map[string]pubRow, error) {
	pubs := map[string]pubRow{}
	for _, id := range rows.catalogIDs {
		c := rows.catalogs[id]
		if !isOriginal(c.public, c.takenFrom) || c.collectionID.Valid {
			p.countReshared(c.public)
			continue
		}
		publicationID := derivedID("uno-publication/catalog/" + c.id)
		pub, err := newPubRow(publicationID, "catalog", catalogSnapshot(publicationID, c),
			shareSource{id: c.id, ownerID: c.ownerID, title: c.name, createdAt: c.createdAt, updatedAt: c.updatedAt})
		if err != nil {
			return nil, err
		}
		p.publications = append(p.publications, pub)
		p.publishedCatalogs++
		pubs[c.id] = pub
	}
	return pubs, nil
}

// countReshared counts a public row that is not published: a linked copy
// made public.
func (p *sharePlan) countReshared(public bool) {
	if public {
		p.reshared++
	}
}

// collectionPub is a collection publication with the source's link hash and
// folders, which its linked copies are compared and paired with.
type collectionPub struct {
	row      pubRow
	linkHash string
	folders  []shareFolder
}

// publishCollections adds a publication for every public collection that
// isn't a linked copy and uses no catalog p subscribes, and returns them by
// source id. It runs after subscribeCatalogs.
func (p *sharePlan) publishCollections(ctx context.Context, tx *sql.Tx, rows shareRows) (map[string]collectionPub, error) {
	subscribed := p.subscribedCatalogs()
	pubs := map[string]collectionPub{}
	for _, id := range rows.collectionIDs {
		c := rows.collections[id]
		if !isOriginal(c.public, c.takenFrom) {
			p.countReshared(c.public)
			continue
		}
		pub, published, err := p.publishOriginal(ctx, tx, c, rows.catalogs, subscribed)
		if err != nil {
			return nil, err
		}
		if published {
			pubs[c.id] = pub
		}
	}
	return pubs, nil
}

// subscribedCatalogs is the id of every catalog p subscribes.
func (p *sharePlan) subscribedCatalogs() map[string]bool {
	ids := map[string]bool{}
	for _, sub := range p.subscriptions {
		if sub.kind == "catalog" {
			ids[sub.copyID] = true
		}
	}
	return ids
}

// publishOriginal adds the publication of original collection c, unless c
// uses a catalog in subscribed, and reports whether it did.
func (p *sharePlan) publishOriginal(ctx context.Context, tx *sql.Tx, c shareCollection, catalogs map[string]shareCatalog, subscribed map[string]bool) (collectionPub, bool, error) {
	folders, err := loadShareFolders(ctx, tx, c.id)
	if err != nil {
		return collectionPub{}, false, err
	}
	if slices.ContainsFunc(referencedCatalogs(folders), func(id string) bool { return subscribed[id] }) {
		p.withheld++
		return collectionPub{}, false, nil
	}
	pub, err := publishCollection(c, folders, catalogs)
	if err != nil {
		return collectionPub{}, false, err
	}
	p.publications = append(p.publications, pub.row)
	p.publishedCollections++
	return pub, true, nil
}

// publishCollection is the publication of collection c, whose folders are
// folders.
func publishCollection(c shareCollection, folders []shareFolder, catalogs map[string]shareCatalog) (collectionPub, error) {
	linkHash, err := collectionLinkHashV2(c, folders, catalogs)
	if err != nil {
		return collectionPub{}, err
	}
	publicationID := derivedID("uno-publication/collection/" + c.id)
	row, err := newPubRow(publicationID, "collection", collectionSnapshot(publicationID, c, folders, catalogs),
		shareSource{id: c.id, ownerID: c.ownerID, title: c.title, createdAt: c.createdAt, updatedAt: c.updatedAt})
	return collectionPub{row: row, linkHash: linkHash, folders: folders}, err
}

// subscribeCatalogs turns every linked listed catalog whose source is
// published into a subscription, and counts every other link as dropped.
func (p *sharePlan) subscribeCatalogs(rows shareRows, pubs map[string]pubRow) {
	for _, id := range rows.catalogIDs {
		c := rows.catalogs[id]
		if !c.takenFrom.Valid || c.collectionID.Valid {
			continue
		}
		source := rows.catalogs[c.takenFrom.String]
		pub, published := pubs[source.id]
		if !published {
			p.catalogLinks.drop(source.takenFrom, source.public)
			continue
		}
		p.subscribe(&p.catalogLinks, pub, "catalog", c.id, c.ownerID, c.createdAt,
			linkState{taken: c.takenHash.String, source: catalogLinkHashV2(source), copyHash: catalogLinkHashV2(c)})
	}
}

// drop counts a link that is not kept: to a source that is itself a copy
// when sourceTakenFrom is set, else to a public source left unpublished when
// sourcePublic is set, and to a private one otherwise.
func (n *linkCounts) drop(sourceTakenFrom sql.NullString, sourcePublic bool) {
	switch {
	case sourceTakenFrom.Valid:
		n.copyOfCopy++
	case sourcePublic:
		n.unpublished++
	default:
		n.private++
	}
}

// linkState is a linked copy's taken_hash, its source's link hash now and
// its own.
type linkState struct{ taken, source, copyHash string }

// takenHash is the subscription taken_hash for a copy in state s of pub, and
// counts which case it fell in. A copy is in step when its source still
// hashes to its taken_hash, or when the copy already equals its source,
// whatever its taken_hash says: a taken_hash computed under an older hash
// rule matches neither, and would read as an update that changes nothing.
func (n *linkCounts) takenHash(s linkState, pub pubRow) string {
	switch {
	case s.taken == s.source:
	case s.copyHash == s.source:
		n.identical++
	default:
		n.outOfStep++
		return outOfStep
	}
	n.inStep++
	return pub.contentHash
}

// subscribe adds copyID's subscription to pub, counted in counts.
func (p *sharePlan) subscribe(counts *linkCounts, pub pubRow, kind, copyID, ownerID, createdAt string, s linkState) {
	p.subscriptions = append(p.subscriptions, subRow{
		id: derivedID("uno-subscription/" + copyID), ownerID: ownerID, publicationID: pub.id, kind: kind,
		copyID: copyID, takenHash: counts.takenHash(s, pub), createdAt: createdAt,
	})
}

// subscribeCollections turns every linked collection whose source is
// published into a subscription, giving its folders and scoped catalogs
// their sub_keys, and counts every other link as dropped.
func (p *sharePlan) subscribeCollections(ctx context.Context, tx *sql.Tx, rows shareRows, pubs map[string]collectionPub) error {
	for _, id := range rows.collectionIDs {
		c := rows.collections[id]
		if !c.takenFrom.Valid {
			continue
		}
		pub, published := pubs[c.takenFrom.String]
		if !published {
			source := rows.collections[c.takenFrom.String]
			p.collectionLinks.drop(source.takenFrom, source.public)
			continue
		}
		if err := p.subscribeCollection(ctx, tx, rows, c, pub); err != nil {
			return err
		}
	}
	return nil
}

// subscribeCollection adds copied collection c's subscription to pub and its
// sub_keys.
func (p *sharePlan) subscribeCollection(ctx context.Context, tx *sql.Tx, rows shareRows, c shareCollection, pub collectionPub) error {
	folders, err := loadShareFolders(ctx, tx, c.id)
	if err != nil {
		return err
	}
	copyHash, err := collectionLinkHashV2(c, folders, rows.catalogs)
	if err != nil {
		return err
	}
	p.subscribe(&p.collectionLinks, pub.row, "collection", c.id, c.ownerID, c.createdAt,
		linkState{taken: c.takenHash.String, source: pub.linkHash, copyHash: copyHash})
	p.keyCopy(rows, c.id, folders, pub)
	return nil
}

// keyCopy gives the folders of copied collection copyID their sub_keys, by
// position against pub's source folders, and its scoped catalogs theirs,
// from the catalog each was taken from.
func (p *sharePlan) keyCopy(rows shareRows, copyID string, folders []shareFolder, pub collectionPub) {
	for i := range min(len(folders), len(pub.folders)) {
		p.folderKeys[folders[i].id] = stableKey(pub.row.id, pub.folders[i].id)
	}
	for _, catalogID := range rows.catalogsByColl[copyID] {
		if takenFrom := rows.catalogs[catalogID].takenFrom; takenFrom.Valid {
			p.catalogKeys[catalogID] = stableKey(pub.row.id, takenFrom.String)
		}
	}
}

// write inserts p's publications and subscriptions and sets its sub_keys,
// after the rebuild. The triggers fill in each publication's search entry and
// subscriber count.
func (p sharePlan) write(ctx context.Context, tx *sql.Tx) error {
	for _, step := range []func() error{
		func() error { return insertPublications(ctx, tx, p.publications) },
		func() error { return insertSubscriptions(ctx, tx, p.subscriptions) },
		func() error { return writeSubKeys(ctx, tx, "catalogs", p.catalogKeys) },
		func() error { return writeSubKeys(ctx, tx, "folders", p.folderKeys) },
	} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// insertPublications inserts each of pubs; see insertPublication.
func insertPublications(ctx context.Context, tx *sql.Tx, pubs []pubRow) error {
	for _, pub := range pubs {
		if err := insertPublication(ctx, tx, pub); err != nil {
			return err
		}
	}
	return nil
}

// insertSubscriptions inserts each of subs; see insertSubscription.
func insertSubscriptions(ctx context.Context, tx *sql.Tx, subs []subRow) error {
	for _, sub := range subs {
		if err := insertSubscription(ctx, tx, sub); err != nil {
			return err
		}
	}
	return nil
}

// insertPublication inserts pub as a live publication.
func insertPublication(ctx context.Context, tx *sql.Tx, pub pubRow) error {
	catalogID, collectionID := sourceColumns(pub.kind, pub.sourceID)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO publications (id, owner_id, kind, catalog_id, collection_id, title, snapshot, content_hash,
		                          catalog_count, folder_count, status, published_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'live', ?, ?)
	`, pub.id, pub.ownerID, pub.kind, catalogID, collectionID, pub.title, pub.snapshot, pub.contentHash,
		pub.catalogCount, pub.folderCount, pub.publishedAt, pub.updatedAt); err != nil {
		return fmt.Errorf("publishing %s %s: %w", pub.kind, pub.sourceID, err)
	}
	return nil
}

// sourceColumns is the catalog_id and collection_id a row of kind naming id
// stores: id in its kind's column, NULL in the other.
func sourceColumns(kind, id string) (any, any) {
	if kind == "catalog" {
		return id, nil
	}
	return nil, id
}

// insertSubscription inserts sub.
func insertSubscription(ctx context.Context, tx *sql.Tx, sub subRow) error {
	catalogID, collectionID := sourceColumns(sub.kind, sub.copyID)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO subscriptions (id, owner_id, publication_id, catalog_id, collection_id, taken_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sub.id, sub.ownerID, sub.publicationID, catalogID, collectionID, sub.takenHash, sub.createdAt); err != nil {
		return fmt.Errorf("subscribing %s %s: %w", sub.kind, sub.copyID, err)
	}
	return nil
}

// writeSubKeys sets sub_key on each row of table keys names. table is one of
// this migration's own literals.
func writeSubKeys(ctx context.Context, tx *sql.Tx, table string, keys map[string]string) error {
	for _, id := range slices.Sorted(maps.Keys(keys)) {
		//nolint:gosec // G202: table is "catalogs" or "folders", never input.
		if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET sub_key = ? WHERE id = ?`, keys[id], id); err != nil {
			return fmt.Errorf("setting %s %s's sub_key: %w", table, id, err)
		}
	}
	return nil
}

// hasLegacyDefault reports whether catalogs still has the legacy is_default
// column, which the rebuild drops.
func hasLegacyDefault(ctx context.Context, tx *sql.Tx) (bool, error) {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('catalogs') WHERE name = 'is_default'`).Scan(&n); err != nil {
		return false, fmt.Errorf("reading catalogs' columns: %w", err)
	}
	return n > 0, nil
}

// notes says what p published, how each kind of link fared, and what the
// rebuild dropped.
func (p sharePlan) notes(legacy bool) []string {
	dropped := "dropped is_public, taken_from and taken_hash from catalogs and collections"
	if legacy {
		dropped += ", and the legacy is_default columns"
	}
	return []string{
		fmt.Sprintf("published %d catalogs and %d collections: every public row that was not a linked copy, as it stood", p.publishedCatalogs, p.publishedCollections),
		fmt.Sprintf("left %d public linked copies unpublished: each became a subscription or a detached copy", p.reshared),
		fmt.Sprintf("left %d public collections unpublished: each uses a catalog that became a subscription, which only its publisher can share", p.withheld),
		p.catalogLinks.note("catalog"),
		p.collectionLinks.note("collection"),
		dropped,
	}
}

// note is one kind of link's counts.
func (n linkCounts) note(kind string) string {
	return fmt.Sprintf("%s links: %d became subscriptions (%d in step, %d of those only because the copy already equals its source; %d out of step); detached %d (%d from a private source, %d from a copy, %d from a public source left unpublished)",
		kind, n.inStep+n.outOfStep, n.inStep, n.identical, n.outOfStep, n.private+n.copyOfCopy+n.unpublished, n.private, n.copyOfCopy, n.unpublished)
}

// publicationsDDL rebuilds catalogs and collections without their sharing
// columns, then adds publications, subscriptions, the sub_key columns and
// every trigger that keeps them consistent. Each rebuild
// creates the new table, copies every row column by column, drops the old one
// and renames the new one into place, so no other table's foreign keys are
// rewritten to follow a rename; then it recreates the table's indexes and
// triggers.
const publicationsDDL = `
CREATE TABLE collections_v3 (
    id                 TEXT    PRIMARY KEY,
    title              TEXT    NOT NULL,
    owner_id           TEXT    NOT NULL REFERENCES profiles(id),
    pin_to_top         INTEGER NOT NULL DEFAULT 0,
    view_mode          TEXT    NOT NULL DEFAULT 'TABBED_GRID',
    show_all_tab       INTEGER NOT NULL DEFAULT 0,
    backdrop_image_url TEXT    NOT NULL DEFAULT '',
    focus_glow_enabled INTEGER NOT NULL DEFAULT 1,
    home_sort_order    INTEGER,                    -- NULL = not on the TV
    version            INTEGER NOT NULL DEFAULT 1, -- +1 on every content write
    pushed_version     INTEGER,                    -- version push read and sent; NULL = never pushed
    created_at         TEXT    NOT NULL,           -- RFC3339 UTC
    updated_at         TEXT    NOT NULL            -- RFC3339 UTC
);
INSERT INTO collections_v3 (id, title, owner_id, pin_to_top, view_mode, show_all_tab, backdrop_image_url,
                            focus_glow_enabled, home_sort_order, version, pushed_version, created_at, updated_at)
SELECT id, title, owner_id, pin_to_top, view_mode, show_all_tab, backdrop_image_url,
       focus_glow_enabled, home_sort_order, version, pushed_version, created_at, updated_at
FROM collections;
DROP TABLE collections;
ALTER TABLE collections_v3 RENAME TO collections;
CREATE INDEX collections_by_owner ON collections (owner_id);

CREATE TABLE catalogs_v3 (
    id              TEXT    PRIMARY KEY,         -- UUID, permanent once selected
    name            TEXT    NOT NULL,
    recipe_hash     TEXT    NOT NULL REFERENCES recipes(hash),
    owner_id        TEXT    NOT NULL REFERENCES profiles(id),
    collection_id   TEXT    REFERENCES collections(id) ON DELETE CASCADE, -- NULL = listed
    home_sort_order INTEGER,                     -- NULL = not on the TV
    show_in_home    INTEGER NOT NULL DEFAULT 1,  -- whether the home row appears when on the TV
    sub_key         TEXT,                        -- in a subscribed collection: its key in the snapshot
    created_at      TEXT    NOT NULL,            -- RFC3339 UTC
    updated_at      TEXT    NOT NULL,            -- RFC3339 UTC
    CHECK (collection_id IS NULL OR home_sort_order IS NULL)
);
INSERT INTO catalogs_v3 (id, name, recipe_hash, owner_id, collection_id, home_sort_order, show_in_home,
                         created_at, updated_at)
SELECT id, name, recipe_hash, owner_id, collection_id, home_sort_order, show_in_home,
       created_at, updated_at
FROM catalogs;
DROP TABLE catalogs;
ALTER TABLE catalogs_v3 RENAME TO catalogs;
CREATE INDEX catalogs_by_owner ON catalogs (owner_id);
CREATE INDEX catalogs_by_collection ON catalogs (collection_id);
CREATE INDEX catalogs_by_recipe ON catalogs (recipe_hash);
CREATE TRIGGER recipes_drop_unused_on_delete AFTER DELETE ON catalogs
WHEN NOT EXISTS (SELECT 1 FROM catalogs WHERE recipe_hash = OLD.recipe_hash)
BEGIN
    DELETE FROM recipes WHERE hash = OLD.recipe_hash;
END;
CREATE TRIGGER recipes_drop_unused_on_repoint AFTER UPDATE OF recipe_hash ON catalogs
WHEN OLD.recipe_hash IS NOT NEW.recipe_hash
 AND NOT EXISTS (SELECT 1 FROM catalogs WHERE recipe_hash = OLD.recipe_hash)
BEGIN
    DELETE FROM recipes WHERE hash = OLD.recipe_hash;
END;

ALTER TABLE folders ADD COLUMN sub_key TEXT;

CREATE TABLE publications (
    id               TEXT    PRIMARY KEY,         -- UUID, kept across republishes
    owner_id         TEXT    NOT NULL REFERENCES profiles(id),
    kind             TEXT    NOT NULL CHECK (kind IN ('catalog', 'collection')),
    catalog_id       TEXT    REFERENCES catalogs(id) ON DELETE SET NULL,    -- the source; NULL once deleted
    collection_id    TEXT    REFERENCES collections(id) ON DELETE SET NULL, -- the source; NULL once deleted
    title            TEXT    NOT NULL,
    snapshot         TEXT    NOT NULL,            -- JSON, format uno-publication
    content_hash     TEXT    NOT NULL,            -- sha256 hex of snapshot
    catalog_count    INTEGER NOT NULL,
    folder_count     INTEGER NOT NULL,
    subscriber_count INTEGER NOT NULL DEFAULT 0,
    status           TEXT    NOT NULL CHECK (status IN ('live', 'withdrawn')),
    published_at     TEXT    NOT NULL,            -- RFC3339 UTC, when first published
    updated_at       TEXT    NOT NULL,            -- RFC3339 UTC
    CHECK (kind = 'catalog' OR catalog_id IS NULL),
    CHECK (kind = 'collection' OR collection_id IS NULL)
);
CREATE UNIQUE INDEX publications_by_catalog ON publications (catalog_id) WHERE catalog_id IS NOT NULL;
CREATE UNIQUE INDEX publications_by_collection ON publications (collection_id) WHERE collection_id IS NOT NULL;

CREATE TABLE subscriptions (
    id             TEXT PRIMARY KEY,
    owner_id       TEXT NOT NULL REFERENCES profiles(id),
    publication_id TEXT NOT NULL REFERENCES publications(id),
    catalog_id     TEXT REFERENCES catalogs(id) ON DELETE CASCADE,    -- the copy, for a catalog
    collection_id  TEXT REFERENCES collections(id) ON DELETE CASCADE, -- the copy, for a collection
    taken_hash     TEXT NOT NULL,                -- the content hash the copy was last written from
    created_at     TEXT NOT NULL,                -- RFC3339 UTC
    UNIQUE (owner_id, publication_id),
    CHECK ((catalog_id IS NULL) <> (collection_id IS NULL))
);
CREATE UNIQUE INDEX subscriptions_by_catalog ON subscriptions (catalog_id) WHERE catalog_id IS NOT NULL;
CREATE UNIQUE INDEX subscriptions_by_collection ON subscriptions (collection_id) WHERE collection_id IS NOT NULL;

CREATE TRIGGER publications_withdraw_on_source_delete AFTER UPDATE OF catalog_id, collection_id ON publications
WHEN NEW.catalog_id IS NULL AND NEW.collection_id IS NULL AND NEW.status = 'live'
BEGIN
    UPDATE publications SET status = 'withdrawn', updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    WHERE id = NEW.id;
END;
CREATE TRIGGER publications_withdraw_on_scope AFTER UPDATE OF collection_id ON catalogs
WHEN NEW.collection_id IS NOT NULL
BEGIN
    UPDATE publications SET status = 'withdrawn', updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    WHERE (catalog_id, status) = (NEW.id, 'live');
END;
CREATE TRIGGER subscriptions_count_on_insert AFTER INSERT ON subscriptions
BEGIN
    UPDATE publications SET subscriber_count = subscriber_count + 1 WHERE id = NEW.publication_id;
END;
CREATE TRIGGER subscriptions_count_on_delete AFTER DELETE ON subscriptions
BEGIN
    UPDATE publications SET subscriber_count = subscriber_count - 1 WHERE id = OLD.publication_id;
END;
`
