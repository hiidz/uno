package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/nuvio"
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

const profileIDKey contextKey = "profileID"

func withProfileID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, profileIDKey, id)
}

func profileIDFrom(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(profileIDKey).(uuid.UUID)
	return id, ok
}

// requireNuvioAuth verifies the request's bearer token against Nuvio's
// JWKS and attaches the resulting account ID (sub) to the request context.
// On failure it responds directly and never calls next.
func (s *Server) requireNuvioAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(authHeader, "Bearer ")
		if !ok || token == "" {
			http.Error(w, "missing or malformed Authorization header", http.StatusUnauthorized)
			return
		}

		claims, err := s.verifier.Verify(r.Context(), token)
		if err != nil {
			switch {
			case errors.Is(err, nuvio.ErrInvalidToken):
				http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			case errors.Is(err, nuvio.ErrJWKSUnavailable):
				http.Error(w, "auth service unavailable", http.StatusBadGateway)
			default:
				http.Error(w, "authentication failed", http.StatusUnauthorized)
			}
			return
		}

		ctx := withNuvioUserID(r.Context(), claims.Sub)
		ctx = withNuvioToken(ctx, token)
		next(w, r.WithContext(ctx))
	}
}

// requireProfile resolves {profileIndex} in the URL, combined with the sub
// already stashed by requireNuvioAuth, into a profile ID — then attaches it
// to the request context. Must be chained after requireNuvioAuth. Lookup
// only: a missing profile is a 404, never provisioned here.
func (s *Server) requireProfile(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sub, ok := nuvioUserIDFrom(r.Context())
		if !ok {
			// Should be unreachable if chained correctly after requireNuvioAuth.
			http.Error(w, "missing authenticated user", http.StatusUnauthorized)
			return
		}

		index, err := strconv.Atoi(r.PathValue("profileIndex"))
		if err != nil || index < 1 || index > 6 {
			http.Error(w, "invalid profile index", http.StatusBadRequest)
			return
		}

		profileID, err := s.vault.GetProfileBySlot(r.Context(), sub, index)
		if err != nil {
			if errors.Is(err, vault.ErrProfileNotFound) {
				http.Error(w, "profile not found", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to resolve profile", http.StatusInternalServerError)
			return
		}

		ctx := withProfileID(r.Context(), profileID)
		next(w, r.WithContext(ctx))
	}
}
