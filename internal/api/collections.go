package api

import (
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

// validateInlineCatalogs runs every folder's New catalog spec through the
// same params validation and fingerprint computation a direct
// POST /catalogs goes through (createUserCatalog), so an inline "copy into
// this collection"/"new inside this collection" create 400s the same way a
// standalone create would, instead of failing deep inside the vault
// transaction with a less specific error.
func (s *Server) validateInlineCatalogs(input *vault.CollectionForm) error {
	for i := range input.Folders {
		for j := range input.Folders[i].Catalogs {
			ref := &input.Folders[i].Catalogs[j]
			if ref.New == nil {
				continue
			}
			if err := s.validateCatalogParams(ref.New.Type, ref.New.Provider, ref.New.Params); err != nil {
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
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	collections, err := s.vault.GetUserCollections(r.Context(), profileID)
	if err != nil {
		log.Printf("listUserCollections: %v", err)
		http.Error(w, "failed to load collections", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, collections)
}

func (s *Server) listCommunityCollections(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	collections, err := s.vault.GetCommunityCollections(r.Context(), profileID)
	if err != nil {
		log.Printf("listCommunityCollections: %v", err)
		http.Error(w, "failed to load collections", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, collections)
}

func (s *Server) takeCollection(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	collectionID, err := uuid.Parse(r.PathValue("collectionID"))
	if err != nil {
		http.Error(w, "invalid collection id", http.StatusBadRequest)
		return
	}

	collection, err := s.vault.TakeCollection(r.Context(), profileID, collectionID)
	if err != nil {
		writeVaultError(w, "takeCollection", err, vault.ErrCollectionNotFound, "collection not found", "failed to take collection")
		return
	}

	writeJSON(w, http.StatusCreated, collection)
}

func (s *Server) duplicateUserCollection(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	collectionID, err := uuid.Parse(r.PathValue("collectionID"))
	if err != nil {
		http.Error(w, "invalid collection id", http.StatusBadRequest)
		return
	}

	collection, err := s.vault.DuplicateCollection(r.Context(), profileID, collectionID)
	if err != nil {
		writeVaultError(w, "duplicateUserCollection", err, vault.ErrCollectionNotFound, "collection not found", "failed to duplicate collection")
		return
	}

	writeJSON(w, http.StatusCreated, collection)
}

func (s *Server) createUserCollection(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	var input vault.CollectionForm
	if !decodeJSON(w, r, &input) {
		return
	}

	if err := s.validateInlineCatalogs(&input); err != nil {
		writeVaultError(w, "createUserCollection", err, nil, "", "failed to create collection")
		return
	}

	collection, err := s.vault.CreateUserCollection(r.Context(), profileID, input)
	if err != nil {
		writeVaultError(w, "createUserCollection", err, nil, "", "failed to create collection")
		return
	}
	writeJSON(w, http.StatusCreated, collection)
}

func (s *Server) updateUserCollection(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	collectionID, err := uuid.Parse(r.PathValue("collectionID"))
	if err != nil {
		http.Error(w, "invalid collection id", http.StatusBadRequest)
		return
	}

	var input vault.CollectionForm
	if !decodeJSON(w, r, &input) {
		return
	}

	if err := s.validateInlineCatalogs(&input); err != nil {
		writeVaultError(w, "updateUserCollection", err, nil, "", "failed to update collection")
		return
	}

	collection, err := s.vault.UpdateUserCollection(r.Context(), profileID, collectionID, input)
	if err != nil {
		writeVaultError(w, "updateUserCollection", err, vault.ErrCollectionNotFound, "collection not found", "failed to update collection")
		return
	}

	writeJSON(w, http.StatusOK, collection)
}

func (s *Server) deleteUserCollection(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	collectionID, err := uuid.Parse(r.PathValue("collectionID"))
	if err != nil {
		http.Error(w, "invalid collection id", http.StatusBadRequest)
		return
	}

	if err := s.vault.DeleteUserCollection(r.Context(), profileID, collectionID); err != nil {
		writeVaultError(w, "deleteUserCollection", err, vault.ErrCollectionNotFound, "collection not found", "failed to delete collection")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listCurrentCollectionSelection(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	collections, err := s.vault.GetCurrentCollectionSelection(r.Context(), profileID)
	if err != nil {
		log.Printf("listCurrentCollectionSelection: %v", err)
		http.Error(w, "failed to load collection selection", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, collections)
}
