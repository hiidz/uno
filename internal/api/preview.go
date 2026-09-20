package api

import (
	"errors"
	"log"
	"net/http"

	"github.com/hiidz/uno/internal/httpx"
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
//
// Genre is optional: a name from the recipe's genre options, narrowing the
// preview the way a collection folder's per-reference genre narrows that row
// on the TV.
type previewRequest struct {
	Type   string `json:"type"`
	Params string `json:"params"`
	Genre  string `json:"genre"`
}

type previewResponse struct {
	// Randomized is true when the recipe shuffles, so preview's page-1
	// result won't match what the TV shows. Returned so the client doesn't
	// need to re-derive it by parsing params.
	Randomized bool                   `json:"randomized"`
	Items      []provider.PreviewItem `json:"items"`
	// TotalResults is TMDB's count of matches across every page these filters
	// return, not just the one page Items carries — the number the builder
	// shows so "20 titles" doesn't get mistaken for the whole answer.
	TotalResults int `json:"total_results"`
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

	items, totalResults, randomized, err := s.provider.PreviewCatalog(r.Context(), input.Type, input.Params, input.Genre)
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

	httpx.WriteJSON(w, http.StatusOK, previewResponse{
		Randomized:   randomized,
		Items:        items,
		TotalResults: totalResults,
	})
}

// genreOptionsRequest is a catalog recipe, the same shape as previewRequest
// minus the genre, so a draft catalog with no id yet has options too.
type genreOptionsRequest struct {
	Type   string `json:"type"`
	Params string `json:"params"`
}

// catalogGenreOptions returns the genres a pick can narrow this recipe by —
// the same provider.GenreExtraOptions list the manifest advertises for the
// catalog, so the collection editor's per-reference genre picker offers
// exactly what the addon path will honour.
func (s *Server) catalogGenreOptions(w http.ResponseWriter, r *http.Request) {
	var input genreOptionsRequest
	if !decodeJSON(w, r, &input) {
		return
	}

	if err := s.validateCatalogParams(input.Type, "tmdb", input.Params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	genres, err := s.provider.GenreExtraOptions(r.Context(), input.Type, input.Params)
	if err != nil {
		if errors.Is(err, provider.ErrInvalidCatalogType) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Printf("catalogGenreOptions: %v", err)
		http.Error(w, "failed to fetch genres", http.StatusBadGateway)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, genres)
}
