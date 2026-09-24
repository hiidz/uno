package api

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/vault"
)

// communityItem is one kind of community item: the path segment carrying an
// original's id, how to name that id, and its not-found sentinel and
// message.
type communityItem struct {
	idParam     string
	idLabel     string
	notFound    error
	notFoundMsg string
}

var (
	catalogItem    = communityItem{"catalogID", "catalog id", vault.ErrCatalogNotFound, "catalog not found"}
	collectionItem = communityItem{"collectionID", "collection id", vault.ErrCollectionNotFound, "collection not found"}
)

// communityCall is one community POST: the kind of item it acts on, the
// handler name its errors are logged under, the message a failure of ours
// answers with, and the status a success answers with.
type communityCall struct {
	item    communityItem
	op      string
	failMsg string
	status  int
}

// serveCommunityCall runs act on the original the path names, for the
// caller's profile, with the params validator every copy of someone else's
// recipe requires, and writes act's result with call.status or its error
// through writeVaultError.
func serveCommunityCall[T any](s *Server, w http.ResponseWriter, r *http.Request, call communityCall,
	act func(ctx context.Context, profileID, sourceID uuid.UUID, validateParams vault.CatalogParamsValidator) (T, error)) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	sourceID, ok := httpx.PathUUID(w, r, call.item.idParam, call.item.idLabel)
	if !ok {
		return
	}

	result, err := act(r.Context(), profileID, sourceID, func(catalogType, catalogProvider, params string) error {
		return s.validateCatalogParams(r.Context(), catalogType, catalogProvider, params)
	})
	if err != nil {
		writeVaultError(w, call.op, err, call.item.notFound, call.item.notFoundMsg, call.failMsg)
		return
	}
	httpx.WriteJSON(w, call.status, result)
}

func (s *Server) updateTakenCatalog(w http.ResponseWriter, r *http.Request) {
	serveCommunityCall(s, w, r, communityCall{catalogItem, "updateTakenCatalog", "failed to update catalog", http.StatusOK},
		s.vault.UpdateTakenCatalog)
}

func (s *Server) duplicateCommunityCatalog(w http.ResponseWriter, r *http.Request) {
	serveCommunityCall(s, w, r, communityCall{catalogItem, "duplicateCommunityCatalog", "failed to duplicate catalog", http.StatusCreated},
		s.vault.DuplicateCommunityCatalog)
}

func (s *Server) updateTakenCollection(w http.ResponseWriter, r *http.Request) {
	serveCommunityCall(s, w, r, communityCall{collectionItem, "updateTakenCollection", "failed to update collection", http.StatusOK},
		s.vault.UpdateTakenCollection)
}

func (s *Server) duplicateCommunityCollection(w http.ResponseWriter, r *http.Request) {
	serveCommunityCall(s, w, r, communityCall{collectionItem, "duplicateCommunityCollection", "failed to duplicate collection", http.StatusCreated},
		s.vault.DuplicateCommunityCollection)
}
