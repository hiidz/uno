package api

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/vault"
)

// pathRow is one kind of row a sharing call names by the id in its path: one
// of the caller's catalogs or collections, or someone's publication, which
// the vault scopes to the caller. It holds the path segment carrying the id,
// how to name that id, and the not-found sentinel and message.
type pathRow struct {
	idParam     string
	idLabel     string
	notFound    error
	notFoundMsg string
}

var (
	catalogRow     = pathRow{"catalogID", "catalog id", vault.ErrCatalogNotFound, "catalog not found"}
	collectionRow  = pathRow{"collectionID", "collection id", vault.ErrCollectionNotFound, "collection not found"}
	publicationRow = pathRow{"publicationID", "publication id", vault.ErrPublicationNotFound, "publication not found"}
)

// sharingCall is one sharing request: the id in its path, the handler name
// its errors are logged under, the message a failure of ours answers with,
// and the status a success answers with.
type sharingCall struct {
	row     pathRow
	op      string
	failMsg string
	status  int
}

// serveSharingCall runs act on the id the path names, for the caller's
// profile, and writes act's result with call.status or its error through
// writeVaultError.
func serveSharingCall[T any](w http.ResponseWriter, r *http.Request, call sharingCall,
	act func(ctx context.Context, profileID, id uuid.UUID) (T, error)) {
	profileID, _ := profileIDFrom(r.Context()) // guaranteed by requireProfile

	id, ok := httpx.PathUUID(w, r, call.row.idParam, call.row.idLabel)
	if !ok {
		return
	}

	result, err := act(r.Context(), profileID, id)
	if err != nil {
		writeVaultError(w, call.op, err, call.row.notFound, call.row.notFoundMsg, call.failMsg)
		return
	}
	httpx.WriteJSON(w, call.status, result)
}

// publishWith adapts a vault publish to serveSharingCall, checking every
// recipe it shares against TMDB with validateCatalogParams, the check a
// catalog save runs.
func publishWith[T any](s *Server, publish func(ctx context.Context, profileID, id uuid.UUID, validateParams vault.CatalogParamsValidator) (T, error)) func(context.Context, uuid.UUID, uuid.UUID) (T, error) {
	return func(ctx context.Context, profileID, id uuid.UUID) (T, error) {
		return publish(ctx, profileID, id, func(catalogType, catalogProvider, params string) error {
			return s.validateCatalogParams(ctx, catalogType, catalogProvider, params)
		})
	}
}

func (s *Server) publishCatalog(w http.ResponseWriter, r *http.Request) {
	serveSharingCall(w, r, sharingCall{catalogRow, "publishCatalog", "failed to publish catalog", http.StatusOK},
		publishWith(s, s.vault.PublishCatalog))
}

func (s *Server) withdrawCatalog(w http.ResponseWriter, r *http.Request) {
	serveSharingCall(w, r, sharingCall{catalogRow, "withdrawCatalog", "failed to withdraw catalog", http.StatusOK},
		s.vault.WithdrawCatalog)
}

func (s *Server) publishCollection(w http.ResponseWriter, r *http.Request) {
	serveSharingCall(w, r, sharingCall{collectionRow, "publishCollection", "failed to publish collection", http.StatusOK},
		publishWith(s, s.vault.PublishCollection))
}

func (s *Server) withdrawCollection(w http.ResponseWriter, r *http.Request) {
	serveSharingCall(w, r, sharingCall{collectionRow, "withdrawCollection", "failed to withdraw collection", http.StatusOK},
		s.vault.WithdrawCollection)
}

// listCommunity answers every live publication Community lists, in one
// call; the SPA searches, filters and sorts them itself.
func (s *Server) listCommunity(w http.ResponseWriter, r *http.Request) {
	listByProfile(w, r, "listCommunity", "failed to load community", s.vault.ListCommunity)
}

func (s *Server) getPublication(w http.ResponseWriter, r *http.Request) {
	serveSharingCall(w, r, sharingCall{publicationRow, "getPublication", "failed to load publication", http.StatusOK},
		s.vault.GetPublication)
}

func (s *Server) subscribe(w http.ResponseWriter, r *http.Request) {
	serveSharingCall(w, r, sharingCall{publicationRow, "subscribe", "failed to subscribe", http.StatusCreated},
		s.vault.Subscribe)
}

func (s *Server) updateSubscription(w http.ResponseWriter, r *http.Request) {
	serveSharingCall(w, r, sharingCall{publicationRow, "updateSubscription", "failed to update", http.StatusOK},
		s.vault.UpdateSubscription)
}

func (s *Server) forkPublication(w http.ResponseWriter, r *http.Request) {
	serveSharingCall(w, r, sharingCall{publicationRow, "forkPublication", "failed to duplicate", http.StatusCreated},
		s.vault.ForkPublication)
}
