package api

import (
	"fmt"
	"net/http"

	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

func (s *Server) listUserCatalogs(w http.ResponseWriter, r *http.Request) {
	listByProfile(w, r, "listUserCatalogs", "failed to load catalogs", s.vault.GetUserCatalogs)
}

func (s *Server) listCommunityCatalogs(w http.ResponseWriter, r *http.Request) {
	listByProfile(w, r, "listCommunityCatalogs", "failed to load catalogs", s.vault.GetCommunityCatalogs)
}

func (s *Server) takeCatalog(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	catalogID, ok := httpx.PathUUID(w, r, "catalogID", "catalog id")
	if !ok {
		return
	}

	catalog, err := s.vault.TakeCatalog(r.Context(), profileID, catalogID,
		func(catalogType, catalogProvider, params string) error {
			return s.validateCatalogParams(r.Context(), catalogType, catalogProvider, params)
		})
	if err != nil {
		writeVaultError(w, "takeCatalog", err, vault.ErrCatalogNotFound, "catalog not found", "failed to take catalog")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, catalog)
}

func (s *Server) createUserCatalog(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	var input vault.CatalogForm
	if !decodeJSON(w, r, &input) {
		return
	}

	if err := s.validateCatalogParams(r.Context(), input.Type, input.Provider, input.Params); err != nil {
		writeVaultError(w, "createUserCatalog", err, nil, "", "failed to create catalog")
		return
	}
	fingerprint, err := provider.Fingerprint(input.Type, input.Provider, input.Params)
	if err != nil {
		writeVaultError(w, "createUserCatalog", fmt.Errorf("%w: %w", vault.ErrInvalidInput, err), nil, "", "failed to create catalog")
		return
	}
	input.Fingerprint = fingerprint

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

	if err := s.validateCatalogParams(r.Context(), input.Type, input.Provider, input.Params); err != nil {
		writeVaultError(w, "updateUserCatalog", err, nil, "", "failed to update catalog")
		return
	}
	fingerprint, err := provider.Fingerprint(input.Type, input.Provider, input.Params)
	if err != nil {
		writeVaultError(w, "updateUserCatalog", fmt.Errorf("%w: %w", vault.ErrInvalidInput, err), nil, "", "failed to update catalog")
		return
	}
	input.Fingerprint = fingerprint

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

func (s *Server) listCurrentCatalogSelection(w http.ResponseWriter, r *http.Request) {
	listByProfile(w, r, "listCurrentCatalogSelection", "failed to load catalog selection", s.vault.GetCurrentCatalogSelection)
}
