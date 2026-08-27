package api

import (
	"errors"
	"log"
	"net/http"

	"github.com/hiidz/uno/internal/provider"
)

// previewRequest is a catalog *recipe*, not a catalog.
//
// Only the two fields that actually vary are on the wire. `provider` is
// hardcoded "tmdb" — the only one with a params shape or any validation — and
// `endpoint` is a pure function of `type`, derived inside the provider package
// (see catalogEndpoint there for why accepting it would be a request-forgery
// surface). Params travels as a JSON-encoded string, matching
// vault.CatalogForm.Params and Catalog.params, so the client can hand over the
// exact string it already holds for a saved catalog with no re-serialisation.
type previewRequest struct {
	Type   string `json:"type"`
	Params string `json:"params"`
}

type previewResponse struct {
	// Randomized is true when the recipe shuffles, so preview's page-1
	// result won't match what the TV shows. Returned so the client doesn't
	// need to re-derive it by parsing params.
	Randomized bool                   `json:"randomized"`
	Items      []provider.PreviewItem `json:"items"`
}

// previewCatalog runs a recipe against TMDB and returns tiles; it saves
// nothing. Takes a raw recipe rather than a saved catalog id so it can
// preview a catalog still being tuned in the builder, which has no id yet.
func (s *Server) previewCatalog(w http.ResponseWriter, r *http.Request) {
	var input previewRequest
	if !decodeJSON(w, r, &input) {
		return
	}

	if err := s.validateCatalogParams(input.Type, "tmdb", input.Params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	items, randomized, err := s.provider.PreviewCatalog(r.Context(), input.Type, input.Params)
	if err != nil {
		if errors.Is(err, provider.ErrInvalidCatalogType) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// 502, never 500: tells the client this was TMDB's fault, so it can
		// degrade to placeholder tiles instead of showing an error.
		log.Printf("previewCatalog: %v", err)
		http.Error(w, "failed to reach TMDB", http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, previewResponse{Randomized: randomized, Items: items})
}
