package api

import (
	"context"
	"net/http"

	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/vault"
)

// validateInlineCatalogs runs every recipe a collection save carries — each
// folder's New catalog spec and each catalog edit — through checkRecipe, the
// same check a direct POST or PUT /catalogs goes through, so a bad inline
// recipe 400s the same way a standalone one would instead of failing deep
// inside the vault transaction with a less specific error. Replaces each
// spec's Params with their canonical form.
func (s *Server) validateInlineCatalogs(ctx context.Context, input *vault.CollectionForm) error {
	if err := s.checkNewCatalogs(ctx, input.Folders); err != nil {
		return err
	}
	return s.checkCatalogEdits(ctx, input.CatalogEdits)
}

// checkNewCatalogs is validateInlineCatalogs for the folders' New entries.
func (s *Server) checkNewCatalogs(ctx context.Context, folders []vault.FolderData) error {
	for i := range folders {
		for j := range folders[i].Catalogs {
			spec := folders[i].Catalogs[j].New
			if spec == nil {
				continue
			}
			params, err := s.checkRecipe(ctx, spec.Type, spec.Provider, spec.Params)
			if err != nil {
				return err
			}
			spec.Params = params
		}
	}
	return nil
}

// checkCatalogEdits is validateInlineCatalogs for the catalog edits.
func (s *Server) checkCatalogEdits(ctx context.Context, edits []vault.ScopedCatalogEdit) error {
	for i := range edits {
		params, err := s.checkRecipe(ctx, edits[i].Type, edits[i].Provider, edits[i].Params)
		if err != nil {
			return err
		}
		edits[i].Params = params
	}
	return nil
}

func (s *Server) duplicateUserCollection(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	collectionID, ok := httpx.PathUUID(w, r, "collectionID", "collection id")
	if !ok {
		return
	}

	collection, err := s.vault.DuplicateCollection(r.Context(), profileID, collectionID)
	if err != nil {
		writeVaultError(w, "duplicateUserCollection", err, vault.ErrCollectionNotFound, "collection not found", "failed to duplicate collection")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, collection)
}

func (s *Server) createUserCollection(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	var input vault.CollectionForm
	if !decodeJSON(w, r, &input) {
		return
	}

	if err := s.validateInlineCatalogs(r.Context(), &input); err != nil {
		writeVaultError(w, "createUserCollection", err, nil, "", "failed to create collection")
		return
	}

	collection, err := s.vault.CreateUserCollection(r.Context(), profileID, input)
	if err != nil {
		writeVaultError(w, "createUserCollection", err, nil, "", "failed to create collection")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, collection)
}

func (s *Server) updateUserCollection(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	collectionID, ok := httpx.PathUUID(w, r, "collectionID", "collection id")
	if !ok {
		return
	}

	var input vault.CollectionForm
	if !decodeJSON(w, r, &input) {
		return
	}

	if err := s.validateInlineCatalogs(r.Context(), &input); err != nil {
		writeVaultError(w, "updateUserCollection", err, nil, "", "failed to update collection")
		return
	}

	collection, err := s.vault.UpdateUserCollection(r.Context(), profileID, collectionID, input)
	if err != nil {
		writeVaultError(w, "updateUserCollection", err, vault.ErrCollectionNotFound, "collection not found", "failed to update collection")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, collection)
}

func (s *Server) deleteUserCollection(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	collectionID, ok := httpx.PathUUID(w, r, "collectionID", "collection id")
	if !ok {
		return
	}

	if err := s.vault.DeleteUserCollection(r.Context(), profileID, collectionID); err != nil {
		writeVaultError(w, "deleteUserCollection", err, vault.ErrCollectionNotFound, "collection not found", "failed to delete collection")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
