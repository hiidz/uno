// Folder writes: the rows and catalog refs a collection's folders are made
// of, shared by the editor save and the tree copy.

package vault

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// folderFrom builds the folder row fd describes: id and sortOrder from the
// caller, every other column straight off fd. The one place that field list
// lives, shared by insertFolder and updateFolder.
func folderFrom(id, collectionID uuid.UUID, sortOrder int, fd FolderData) Folder {
	return Folder{
		ID:              id,
		CollectionID:    collectionID,
		Title:           fd.Title,
		SortOrder:       sortOrder,
		TileShape:       fd.TileShape,
		HideTitle:       fd.HideTitle,
		CoverEmoji:      fd.CoverEmoji,
		CoverImageURL:   fd.CoverImageURL,
		FocusGIFURL:     fd.FocusGIFURL,
		FocusGIFEnabled: fd.FocusGIFEnabled,
		HeroBackdropURL: fd.HeroBackdropURL,
		HeroVideoURL:    fd.HeroVideoURL,
		TitleLogoURL:    fd.TitleLogoURL,
	}
}

// insertFolder inserts fd as a new folder under collectionID at sortOrder
// and returns the resulting row (with a freshly generated ID).
func insertFolder(ctx context.Context, tx *sql.Tx, collectionID uuid.UUID, sortOrder int, fd FolderData) (Folder, error) {
	f := folderFrom(uuid.New(), collectionID, sortOrder, fd)

	_, err := tx.ExecContext(ctx, `
		INSERT INTO folders (id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url,
		                     focus_gif_url, focus_gif_enabled, hero_backdrop_url, hero_video_url, title_logo_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, f.ID.String(), f.CollectionID.String(), f.Title, f.SortOrder, f.TileShape, f.HideTitle, f.CoverEmoji, f.CoverImageURL,
		f.FocusGIFURL, f.FocusGIFEnabled, f.HeroBackdropURL, f.HeroVideoURL, f.TitleLogoURL)
	if err != nil {
		return Folder{}, fmt.Errorf("inserting folder: %w", err)
	}
	return f, nil
}

// updateFolder rewrites the folder identified by id in place — the same
// columns insertFolder writes, minus the immutable ones — and returns the
// resulting row.
func updateFolder(ctx context.Context, tx *sql.Tx, id, collectionID uuid.UUID, sortOrder int, fd FolderData) (Folder, error) {
	f := folderFrom(id, collectionID, sortOrder, fd)

	// collection_id is in the WHERE as well as the id: removedFolderIDs has
	// already rejected any incoming folder that belongs elsewhere, but that
	// guard is a separate call, and the zero-rows check below makes this
	// statement reject a folder from another collection on its own if it ever
	// runs without it.
	result, err := tx.ExecContext(ctx, `
		UPDATE folders
		SET title = ?, sort_order = ?, tile_shape = ?, hide_title = ?, cover_emoji = ?, cover_image_url = ?,
		    focus_gif_url = ?, focus_gif_enabled = ?, hero_backdrop_url = ?, hero_video_url = ?, title_logo_url = ?
		WHERE id = ? AND collection_id = ?
	`, f.Title, f.SortOrder, f.TileShape, f.HideTitle, f.CoverEmoji, f.CoverImageURL,
		f.FocusGIFURL, f.FocusGIFEnabled, f.HeroBackdropURL, f.HeroVideoURL, f.TitleLogoURL,
		f.ID.String(), f.CollectionID.String())
	if err != nil {
		return Folder{}, fmt.Errorf("updating folder: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return Folder{}, fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return Folder{}, fmt.Errorf("%w: folder %s does not belong to this collection", ErrInvalidInput, f.ID)
	}
	return f, nil
}

// rewriteFolderCatalogRefs wipes and rewrites folder_catalogs for folderID,
// in order, from already-resolved refs: writeFolderCatalogRefs below resolves
// its inline-create entries first and then writes them here.
func rewriteFolderCatalogRefs(ctx context.Context, tx *sql.Tx, folderID uuid.UUID, refs []FolderRef) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM folder_catalogs WHERE folder_id = ?`, folderID.String()); err != nil {
		return fmt.Errorf("clearing folder catalog refs: %w", err)
	}
	for j, ref := range refs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre)
			VALUES (?, ?, ?, ?)
		`, folderID.String(), ref.CatalogID.String(), j, ref.Genre); err != nil {
			return fmt.Errorf("inserting folder catalog ref: %w", err)
		}
	}
	return nil
}

// existingRefIDs returns the CatalogID of every ref in refs that references
// an existing catalog, skipping New entries — they aren't rows yet, so
// there's nothing for validateFolderRefs to check.
func existingRefIDs(refs []FolderCatalogRef) []uuid.UUID {
	var ids []uuid.UUID
	for _, ref := range refs {
		if ref.CatalogID != nil {
			ids = append(ids, *ref.CatalogID)
		}
	}
	return ids
}

// existingFolderRefIDs is existingRefIDs across every folder of a save, so
// one validateFolderRefs query can check them all up front.
func existingFolderRefIDs(folders []FolderData) []uuid.UUID {
	var ids []uuid.UUID
	for _, fd := range folders {
		ids = append(ids, existingRefIDs(fd.Catalogs)...)
	}
	return ids
}

// resolveFolderCatalogRef returns ref's catalog id, inserting a fresh
// catalog scoped to collectionID first when ref.New is set — owned by
// profileID, never public, never home-eligible, in the caller's own
// transaction. This is the one place a "copy into this collection"/"new
// inside this collection" catalog is ever written: atomic with the folder
// write that references it, so an edit discarded instead of saved never
// created one at all. created maps each New.Key already resolved in this
// save to its catalog id, so a Key's later entries reuse that catalog.
// spec.TakenFrom becomes the row's taken_from; only a collection Take sets it.
func resolveFolderCatalogRef(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, ref FolderCatalogRef, created map[string]uuid.UUID) (uuid.UUID, error) {
	if ref.CatalogID != nil {
		return *ref.CatalogID, nil
	}

	spec := ref.New
	if id, ok := created[spec.Key]; ok {
		return id, nil
	}
	now := time.Now().UTC()
	c := Catalog{
		ID:           uuid.New(),
		Type:         spec.Type,
		Name:         spec.Name,
		Provider:     spec.Provider,
		Params:       spec.Params,
		OwnerID:      profileID,
		CollectionID: &collectionID,
		TakenFrom:    spec.TakenFrom,
		Fingerprint:  spec.Fingerprint,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := insertCatalog(ctx, tx, c); err != nil {
		return uuid.Nil, err
	}
	created[spec.Key] = c.ID
	return c.ID, nil
}

// writeFolderCatalogRefs wipes and rewrites folder_catalogs for folderID, in
// order, resolving each ref via resolveFolderCatalogRef first. Safe to call
// for a brand-new folder too — the delete is then a no-op. Refs are cheap
// and small compared to folders, so delete-and-reinsert beats diffing them.
// Returns the resolved refs in order, for the caller's response shape and
// final catalog fetch. created is shared across every folder of one save;
// see resolveFolderCatalogRef.
func writeFolderCatalogRefs(ctx context.Context, tx *sql.Tx, profileID, collectionID, folderID uuid.UUID, refs []FolderCatalogRef, created map[string]uuid.UUID) ([]FolderRef, error) {
	resolved := make([]FolderRef, len(refs))
	for j, ref := range refs {
		catalogID, err := resolveFolderCatalogRef(ctx, tx, profileID, collectionID, ref, created)
		if err != nil {
			return nil, err
		}
		resolved[j] = FolderRef{CatalogID: catalogID, Genre: strings.TrimSpace(ref.Genre)}
	}
	if err := rewriteFolderCatalogRefs(ctx, tx, folderID, resolved); err != nil {
		return nil, err
	}
	return resolved, nil
}

// removedFolderIDs returns the ids of collectionID's folders that incoming
// drops, confirming along the way that every folder incoming claims to keep
// belongs to this collection.
func removedFolderIDs(ctx context.Context, tx *sql.Tx, collectionID uuid.UUID, incoming []FolderData) ([]uuid.UUID, error) {
	existingIDs, err := queryUUIDs(ctx, tx, "folder id",
		`SELECT id FROM folders WHERE collection_id = ?`, collectionID.String())
	if err != nil {
		return nil, err
	}
	existing := make(map[uuid.UUID]bool, len(existingIDs))
	for _, id := range existingIDs {
		existing[id] = true
	}

	keep := make(map[uuid.UUID]bool, len(incoming))
	for _, fd := range incoming {
		if fd.ID == nil {
			continue
		}
		if !existing[*fd.ID] {
			return nil, fmt.Errorf("%w: folder %s does not belong to this collection", ErrInvalidInput, *fd.ID)
		}
		keep[*fd.ID] = true
	}

	var removed []uuid.UUID
	for _, id := range existingIDs {
		if !keep[id] {
			removed = append(removed, id)
		}
	}
	return removed, nil
}

// deleteRemovedFolders deletes the folders of collectionID that incoming
// drops; see removedFolderIDs.
func deleteRemovedFolders(ctx context.Context, tx *sql.Tx, collectionID uuid.UUID, incoming []FolderData) error {
	removed, err := removedFolderIDs(ctx, tx, collectionID, incoming)
	if err != nil {
		return err
	}
	return deleteFolders(ctx, tx, removed)
}

// deleteFolders removes the given folders. ON DELETE CASCADE on
// folder_catalogs.folder_id handles their refs.
func deleteFolders(ctx context.Context, tx *sql.Tx, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders, args := buildInClause(ids)
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM folders WHERE id IN (%s)`, placeholders), args...); err != nil {
		return fmt.Errorf("deleting removed folders: %w", err)
	}
	return nil
}

// insertFolders writes folders as the folder set of collectionID, a
// collection created in this same transaction: every entry is a new folder
// row, whatever ID it carries, and its catalog refs are written after it.
// Returns the folders in the response's nested shape, plus every catalog id
// they reference with repeats included.
func insertFolders(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, folders []FolderData) ([]FolderWithCatalogs, []uuid.UUID, error) {
	out := make([]FolderWithCatalogs, len(folders))
	var allCatalogIDs []uuid.UUID
	created := map[string]uuid.UUID{}
	for i, fd := range folders {
		f, err := insertFolder(ctx, tx, collectionID, i, fd)
		if err != nil {
			return nil, nil, err
		}
		refs, err := writeFolderCatalogRefs(ctx, tx, profileID, collectionID, f.ID, fd.Catalogs, created)
		if err != nil {
			return nil, nil, err
		}

		out[i] = FolderWithCatalogs{Folder: f, Refs: refs}
		allCatalogIDs = append(allCatalogIDs, out[i].CatalogIDs()...)
	}
	return out, allCatalogIDs, nil
}

// writeFolderSet checks every existing catalog incoming references is usable
// in collectionID, writes incoming as its complete folder set
// (upsertFolders), then deletes any catalog scoped to collectionID that no
// folder references any more. Returns what upsertFolders returns.
func writeFolderSet(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, incoming []FolderData) ([]FolderWithCatalogs, []uuid.UUID, error) {
	if err := validateFolderRefs(ctx, tx, profileID, &collectionID, existingFolderRefIDs(incoming)); err != nil {
		return nil, nil, err
	}
	folders, allCatalogIDs, err := upsertFolders(ctx, tx, profileID, collectionID, incoming)
	if err != nil {
		return nil, nil, err
	}
	if err := deleteOrphanedScopedCatalogs(ctx, tx, collectionID); err != nil {
		return nil, nil, err
	}
	return folders, allCatalogIDs, nil
}

// upsertFolders writes incoming as collectionID's complete folder set, in
// order: an entry carrying an ID updates that folder in place, one without
// inserts a new row, and either way its catalog refs are rewritten. Returns
// the folders in the response's nested shape, plus every catalog id they
// reference with repeats included.
func upsertFolders(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, incoming []FolderData) ([]FolderWithCatalogs, []uuid.UUID, error) {
	folders := make([]FolderWithCatalogs, len(incoming))
	var allCatalogIDs []uuid.UUID
	created := map[string]uuid.UUID{}
	for i, fd := range incoming {
		var f Folder
		var err error
		if fd.ID != nil {
			f, err = updateFolder(ctx, tx, *fd.ID, collectionID, i, fd)
		} else {
			f, err = insertFolder(ctx, tx, collectionID, i, fd)
		}
		if err != nil {
			return nil, nil, err
		}

		refs, err := writeFolderCatalogRefs(ctx, tx, profileID, collectionID, f.ID, fd.Catalogs, created)
		if err != nil {
			return nil, nil, err
		}

		folders[i] = FolderWithCatalogs{Folder: f, Refs: refs}
		allCatalogIDs = append(allCatalogIDs, folders[i].CatalogIDs()...)
	}
	return folders, allCatalogIDs, nil
}
