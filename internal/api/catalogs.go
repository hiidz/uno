package api

import (
	"net/http"

	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/vault"
)

func (s *Server) listUserCatalogs(w http.ResponseWriter, r *http.Request) {
	listByProfile(w, r, "listUserCatalogs", "failed to load catalogs", s.vault.GetUserCatalogs)
}

func (s *Server) createUserCatalog(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	var input vault.CatalogForm
	if !decodeJSON(w, r, &input) {
		return
	}

	params, err := s.checkRecipe(r.Context(), input.Type, input.Provider, input.Params)
	if err != nil {
		writeVaultError(w, "createUserCatalog", err, nil, "", "failed to create catalog")
		return
	}
	input.Params = params

	catalog, err := s.vault.CreateUserCatalog(r.Context(), profileID, input)
	if err != nil {
		writeVaultError(w, "createUserCatalog", err, nil, "", "failed to create catalog")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, catalog)
}

func (s *Server) updateUserCatalog(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	catalogID, ok := httpx.PathUUID(w, r, "catalogID", "catalog id")
	if !ok {
		return
	}

	var input vault.CatalogForm
	if !decodeJSON(w, r, &input) {
		return
	}

	params, err := s.checkRecipe(r.Context(), input.Type, input.Provider, input.Params)
	if err != nil {
		writeVaultError(w, "updateUserCatalog", err, nil, "", "failed to update catalog")
		return
	}
	input.Params = params

	catalog, err := s.vault.UpdateUserCatalog(r.Context(), profileID, catalogID, input)
	if err != nil {
		writeVaultError(w, "updateUserCatalog", err, vault.ErrCatalogNotFound, "catalog not found", "failed to update catalog")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, catalog)
}

func (s *Server) deleteUserCatalog(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	catalogID, ok := httpx.PathUUID(w, r, "catalogID", "catalog id")
	if !ok {
		return
	}

	if err := s.vault.DeleteUserCatalog(r.Context(), profileID, catalogID); err != nil {
		writeVaultError(w, "deleteUserCatalog", err, vault.ErrCatalogNotFound, "catalog not found", "failed to delete catalog")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
