package api

import (
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

func (s *Server) listUserCatalogs(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	catalogs, err := s.vault.GetUserCatalogs(r.Context(), profileID)
	if err != nil {
		log.Printf("listUserCatalogs: %v", err)
		http.Error(w, "failed to load catalogs", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, catalogs)
}

func (s *Server) listCommunityCatalogs(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	catalogs, err := s.vault.GetCommunityCatalogs(r.Context(), profileID)
	if err != nil {
		log.Printf("listCommunityCatalogs: %v", err)
		http.Error(w, "failed to load catalogs", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, catalogs)
}

func (s *Server) takeCatalog(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	catalogID, err := uuid.Parse(r.PathValue("catalogID"))
	if err != nil {
		http.Error(w, "invalid catalog id", http.StatusBadRequest)
		return
	}

	catalog, err := s.vault.TakeCatalog(r.Context(), profileID, catalogID)
	if err != nil {
		writeVaultError(w, "takeCatalog", err, vault.ErrCatalogNotFound, "catalog not found", "failed to take catalog")
		return
	}
	writeJSON(w, http.StatusCreated, catalog)
}

func (s *Server) createUserCatalog(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	var input vault.CatalogForm
	if !decodeJSON(w, r, &input) {
		return
	}

	if err := s.validateCatalogParams(input.Type, input.Provider, input.Params); err != nil {
		writeVaultError(w, "createUserCatalog", err, nil, "", "failed to create catalog")
		return
	}
	fingerprint, err := provider.Fingerprint(input.Type, input.Provider, input.Params)
	if err != nil {
		writeVaultError(w, "createUserCatalog", fmt.Errorf("%w: %v", vault.ErrInvalidInput, err), nil, "", "failed to create catalog")
		return
	}
	input.Fingerprint = fingerprint

	catalog, err := s.vault.CreateUserCatalog(r.Context(), profileID, input)
	if err != nil {
		writeVaultError(w, "createUserCatalog", err, nil, "", "failed to create catalog")
		return
	}
	writeJSON(w, http.StatusCreated, catalog)
}

func (s *Server) updateUserCatalog(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	catalogID, err := uuid.Parse(r.PathValue("catalogID"))
	if err != nil {
		http.Error(w, "invalid catalog id", http.StatusBadRequest)
		return
	}

	var input vault.CatalogForm
	if !decodeJSON(w, r, &input) {
		return
	}

	if err := s.validateCatalogParams(input.Type, input.Provider, input.Params); err != nil {
		writeVaultError(w, "updateUserCatalog", err, vault.ErrCatalogNotFound, "catalog not found", "failed to update catalog")
		return
	}
	fingerprint, err := provider.Fingerprint(input.Type, input.Provider, input.Params)
	if err != nil {
		writeVaultError(w, "updateUserCatalog", fmt.Errorf("%w: %v", vault.ErrInvalidInput, err), vault.ErrCatalogNotFound, "catalog not found", "failed to update catalog")
		return
	}
	input.Fingerprint = fingerprint

	catalog, err := s.vault.UpdateUserCatalog(r.Context(), profileID, catalogID, input)
	if err != nil {
		writeVaultError(w, "updateUserCatalog", err, vault.ErrCatalogNotFound, "catalog not found", "failed to update catalog")
		return
	}
	writeJSON(w, http.StatusOK, catalog)
}

func (s *Server) deleteUserCatalog(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	catalogID, err := uuid.Parse(r.PathValue("catalogID"))
	if err != nil {
		http.Error(w, "invalid catalog id", http.StatusBadRequest)
		return
	}

	if err := s.vault.DeleteUserCatalog(r.Context(), profileID, catalogID); err != nil {
		writeVaultError(w, "deleteUserCatalog", err, vault.ErrCatalogNotFound, "catalog not found", "failed to delete catalog")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listCurrentCatalogSelection(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	catalogs, err := s.vault.GetCurrentCatalogSelection(r.Context(), profileID)
	if err != nil {
		log.Printf("listCurrentCatalogSelection: %v", err)
		http.Error(w, "failed to load catalog selection", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, catalogs)
}
