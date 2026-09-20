package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

// validateInlineCatalogs runs every folder's New catalog spec through the
// same params validation and fingerprint computation a direct
// POST /catalogs goes through (createUserCatalog), so an inline "copy into
// this collection"/"new inside this collection" create 400s the same way a
// standalone create would, instead of failing deep inside the vault
// transaction with a less specific error.
func (s *Server) validateInlineCatalogs(ctx context.Context, input *vault.CollectionForm) error {
	for i := range input.Folders {
		for j := range input.Folders[i].Catalogs {
			ref := &input.Folders[i].Catalogs[j]
			if ref.New == nil {
				continue
			}
			if err := s.validateCatalogParams(ctx, ref.New.Type, ref.New.Provider, ref.New.Params); err != nil {
				return err
			}
			fingerprint, err := provider.Fingerprint(ref.New.Type, ref.New.Provider, ref.New.Params)
			if err != nil {
				return fmt.Errorf("%w: %v", vault.ErrInvalidInput, err)
			}
			ref.New.Fingerprint = fingerprint
		}
	}
	return nil
}

func (s *Server) listUserCollections(w http.ResponseWriter, r *http.Request) {
	listByProfile(w, r, "listUserCollections", "failed to load collections", s.vault.GetUserCollections)
}

func (s *Server) listCommunityCollections(w http.ResponseWriter, r *http.Request) {
	listByProfile(w, r, "listCommunityCollections", "failed to load collections", s.vault.GetCommunityCollections)
}

func (s *Server) takeCollection(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	collectionID, ok := httpx.PathUUID(w, r, "collectionID", "collection id")
	if !ok {
		return
	}

	collection, err := s.vault.TakeCollection(r.Context(), profileID, collectionID,
		func(catalogType, catalogProvider, params string) error {
			return s.validateCatalogParams(r.Context(), catalogType, catalogProvider, params)
		})
	if err != nil {
		writeVaultError(w, "takeCollection", err, vault.ErrCollectionNotFound, "collection not found", "failed to take collection")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, collection)
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

func (s *Server) listCurrentCollectionSelection(w http.ResponseWriter, r *http.Request) {
	listByProfile(w, r, "listCurrentCollectionSelection", "failed to load collection selection", s.vault.GetCurrentCollectionSelection)
}
