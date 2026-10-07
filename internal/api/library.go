package api

import (
	"net/http"

	"github.com/hiidz/uno/internal/httpx"
)

// getLibrary serves GET /api/p/{profileIndex}/library: the profile's listed
// catalogs, its collections with their folders and catalogs, and what a push
// of the Home as Uno stores it would change in Nuvio (vault.PendingPush), read
// in one snapshot. The builder adds its own unpushed Home edits to the pending
// list.
func (s *Server) getLibrary(w http.ResponseWriter, r *http.Request) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	lib, err := s.vault.GetLibrary(r.Context(), profileID)
	if err != nil {
		serverError(w, "getLibrary", err, "failed to load library")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, lib)
}
