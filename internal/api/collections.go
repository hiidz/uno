package api

import (
	"log"
	"net/http"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/vault"
)

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
	collections, err := s.vault.GetCommunityCollections(r.Context())
	if err != nil {
		log.Printf("listCommunityCollections: %v", err)
		http.Error(w, "failed to load collections", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, collections)
}

func (s *Server) createUserCollection(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	var input vault.CollectionForm
	if !decodeJSON(w, r, &input) {
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
