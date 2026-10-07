package api

import (
	"log"
	"net/http"

	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/vault"
)

// library is everything the builder shows for a profile, in one read.
type library struct {
	Catalogs    []vault.Catalog               `json:"catalogs"`
	Collections []vault.CollectionWithFolders `json:"collections"`
	Pending     []vault.PendingChange         `json:"pending"`
}

// getLibrary serves GET /api/p/{profileIndex}/library: the profile's listed
// catalogs, its collections with their folders and catalogs, and what a push
// of the Home as Uno stores it would change in Nuvio (vault.PendingPush). The
// builder adds its own unpushed Home edits to the pending list.
func (s *Server) getLibrary(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	var lib library
	var err error
	if lib.Catalogs, err = s.vault.GetUserCatalogs(r.Context(), profileID); err == nil {
		if lib.Collections, err = s.vault.GetUserCollections(r.Context(), profileID); err == nil {
			lib.Pending, err = s.vault.PendingPush(r.Context(), profileID)
		}
	}
	if err != nil {
		log.Printf("getLibrary: %v", err)
		http.Error(w, "failed to load library", http.StatusInternalServerError)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, lib)
}
