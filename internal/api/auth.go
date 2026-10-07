package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

type contextKey string

const nuvioUserIDKey contextKey = "nuvioUserID"

func withNuvioUserID(ctx context.Context, sub string) context.Context {
	return context.WithValue(ctx, nuvioUserIDKey, sub)
}

func nuvioUserIDFrom(ctx context.Context) (string, bool) {
	sub, ok := ctx.Value(nuvioUserIDKey).(string)
	return sub, ok
}

const nuvioTokenKey contextKey = "nuvioToken"

func withNuvioToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, nuvioTokenKey, token)
}

func nuvioTokenFrom(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(nuvioTokenKey).(string)
	return token, ok
}

const profileKey contextKey = "profile"

func withProfile(ctx context.Context, p vault.Profile) context.Context {
	return context.WithValue(ctx, profileKey, p)
}

func profileFrom(ctx context.Context) (vault.Profile, bool) {
	p, ok := ctx.Value(profileKey).(vault.Profile)
	return p, ok
}

// profileIDFrom is the id of the profile requireProfile resolved.
func profileIDFrom(ctx context.Context) (uuid.UUID, bool) {
	p, ok := profileFrom(ctx)
	return p.ID, ok
}

// requireNuvioAuth verifies the request's bearer token against Nuvio's
// JWKS, checks its account against the access policy (authenticate), and
// attaches the account ID (sub) and the token to the request context, with
// the account's own TMDB key for any TMDB call the request makes (on a server
// where each account brings one; read only if TMDB is reached). On
// failure it responds directly and never calls next.
func (s *Server) requireNuvioAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, claims, refusal := s.authenticate(r)
		if refusal != nil {
			http.Error(w, refusal.msg, refusal.status)
			return
		}
		ctx := withNuvioUserID(r.Context(), claims.Sub)
		ctx = withNuvioToken(ctx, token)
		ctx = provider.WithKeySource(ctx, s.keys.ForAccount(ctx, claims.Sub))
		next(w, r.WithContext(ctx))
	}
}

// requireProfileAuth is requireNuvioAuth followed by requireProfile — the
// chain every profile-scoped route uses, named once so the route table
// doesn't spell the nesting out per line. requireProfile depends on what
// requireNuvioAuth stashes, so the order is not interchangeable.
func (s *Server) requireProfileAuth(next http.HandlerFunc) http.HandlerFunc {
	return s.requireNuvioAuth(s.requireProfile(next))
}

// requireProfile resolves {profileIndex} in the URL, combined with the sub
// already stashed by requireNuvioAuth, into the vault profile — then attaches
// it to the request context. Must be chained after requireNuvioAuth. Lookup
// only: a missing profile is a 404, never provisioned here.
func (s *Server) requireProfile(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sub, _ := nuvioUserIDFrom(r.Context()) // guaranteed by requireNuvioAuth

		index, err := strconv.Atoi(r.PathValue("profileIndex"))
		if err != nil || index < 1 || index > 6 {
			http.Error(w, "invalid profile index", http.StatusBadRequest)
			return
		}

		profile, err := s.vault.GetProfileBySlot(r.Context(), sub, index)
		if err != nil {
			if errors.Is(err, vault.ErrProfileNotFound) {
				// The builder tells this 404 from a route's own by its code
				// (web/src/api/http.ts).
				httpx.WriteJSON(w, http.StatusNotFound, codedError{Error: "profile not found", Code: codeProfileNotFound})
				return
			}
			log.Printf("requireProfile: %v", err)
			http.Error(w, "failed to resolve profile", http.StatusInternalServerError)
			return
		}

		ctx := withProfile(r.Context(), profile)
		next(w, r.WithContext(ctx))
	}
}
