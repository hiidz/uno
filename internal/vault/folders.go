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

// insertFolder inserts fd as a new folder under collectionID at sortOrder
// and returns the resulting row (with a freshly generated ID).
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

	_, err := tx.ExecContext(ctx, `
		UPDATE folders
		SET title = ?, sort_order = ?, tile_shape = ?, hide_title = ?, cover_emoji = ?, cover_image_url = ?,
		    focus_gif_url = ?, focus_gif_enabled = ?, hero_backdrop_url = ?, hero_video_url = ?, title_logo_url = ?
		WHERE id = ?
	`, f.Title, f.SortOrder, f.TileShape, f.HideTitle, f.CoverEmoji, f.CoverImageURL,
		f.FocusGIFURL, f.FocusGIFEnabled, f.HeroBackdropURL, f.HeroVideoURL, f.TitleLogoURL, f.ID.String())
	if err != nil {
		return Folder{}, fmt.Errorf("updating folder: %w", err)
	}
	return f, nil
}

// rewriteFolderCatalogRefs wipes and rewrites folder_catalogs for folderID,
// in order, from already-resolved refs. Both ref-writing paths end here: the
// tree copy (copyCollectionTree, for Take/Duplicate), which has already
// minted or mapped every id it needs, and writeFolderCatalogRefs below,
// which resolves its inline-create entries first.
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

// resolveFolderCatalogRef returns ref's catalog id, inserting a fresh
// catalog scoped to collectionID first when ref.New is set — owned by
// profileID, never public, never home-eligible, in the caller's own
// transaction. This is the one place a "copy into this collection"/"new
// inside this collection" catalog is ever written: atomic with the folder
// write that references it, so an edit discarded instead of saved never
// created one at all. created maps each New.Key already resolved in this
// save to its catalog id, so a Key's later entries reuse that catalog.
func resolveFolderCatalogRef(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, ref FolderCatalogRef, created map[string]uuid.UUID) (uuid.UUID, error) {
	if ref.CatalogID != nil {
		return *ref.CatalogID, nil
	}

	spec := ref.New
	if id, ok := created[spec.Key]; ok {
		return id, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.New()
	_, err := tx.ExecContext(ctx, `
		INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, is_default,
		                       collection_id, fingerprint, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id.String(), spec.Type, spec.Name, spec.Provider, spec.Params, profileID.String(), false, false,
		collectionID.String(), spec.Fingerprint, now, now)
	if err != nil {
		return uuid.Nil, fmt.Errorf("inserting scoped catalog: %w", err)
	}
	created[spec.Key] = id
	return id, nil
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
