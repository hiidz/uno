package api

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/tmdbkey"
	"github.com/hiidz/uno/internal/vault"
)

// serverConfig is what the SPA is told of how this server is set up: whether
// every account shares one TMDB key ("shared") or brings its own
// ("per-account").
type serverConfig struct {
	TMDBKeyMode string `json:"tmdb_key_mode"`
}

// config answers GET /api/config. It needs no sign-in: it says nothing about
// any account.
func (s *Server) config(w http.ResponseWriter, _ *http.Request) {
	mode := "shared"
	if s.keys != nil {
		mode = "per-account"
	}
	httpx.WriteJSON(w, http.StatusOK, serverConfig{TMDBKeyMode: mode})
}

// tmdbKeyStatus is all the SPA is ever told of the signed-in account's TMDB
// key: whether it has saved one, and that key's last four characters.
type tmdbKeyStatus struct {
	Set   bool   `json:"set"`
	Last4 string `json:"last4,omitempty"`
}

// perAccountKeys runs next only on a server where each account brings its own
// TMDB key; on one with a shared key, the key routes are a 404.
func (s *Server) perAccountKeys(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.keys == nil {
			httpx.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		next(w, r)
	}
}

// getTMDBKey answers GET /api/account/tmdb-key: whether the signed-in account
// has saved a key, and its last four characters.
func (s *Server) getTMDBKey(w http.ResponseWriter, r *http.Request) {
	sub, _ := nuvioUserIDFrom(r.Context()) // guaranteed by requireNuvioAuth
	key, err := s.vault.AccountKey(r.Context(), sub, tmdbkey.Provider)
	if errors.Is(err, vault.ErrNoAccountKey) {
		httpx.WriteJSON(w, http.StatusOK, tmdbKeyStatus{})
		return
	}
	if err != nil {
		serverError(w, "getTMDBKey", err, "failed to read your TMDB key")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, tmdbKeyStatus{Set: true, Last4: key.Last4})
}

type putTMDBKeyRequest struct {
	Key string `json:"key"`
}

// putTMDBKey answers PUT /api/account/tmdb-key: it saves the signed-in
// account's key, replacing any it had, once the key has the shape of a TMDB
// API key and TMDB accepts it (one call). Nothing is saved otherwise.
func (s *Server) putTMDBKey(w http.ResponseWriter, r *http.Request) {
	var input putTMDBKeyRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	key, err := tmdbkey.Clean(input.Key)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, keyShapeMessage(err))
		return
	}
	if err := s.provider.CheckKey(r.Context(), key); err != nil {
		writeKeyCheckError(w, err)
		return
	}
	status, err := s.storeTMDBKey(r.Context(), key)
	if err != nil {
		serverError(w, "putTMDBKey", err, "failed to save your TMDB key")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, status)
}

// keyShapeMessage words a pasted key that isn't a TMDB API key.
func keyShapeMessage(err error) string {
	if errors.Is(err, tmdbkey.ErrReadAccessToken) {
		return "That's the Read Access Token. Paste the shorter API Key from the same page."
	}
	return "A TMDB API Key is 32 characters, 0–9 and a–f."
}

// writeKeyCheckError answers a key TMDB refused (400: the key as sent is at
// fault) or couldn't check (502). Neither saves anything.
func writeKeyCheckError(w http.ResponseWriter, err error) {
	if errors.Is(err, provider.ErrKeyRejected) {
		httpx.WriteError(w, http.StatusBadRequest, "TMDB didn't accept this key. Check you copied the API Key.")
		return
	}
	log.Printf("putTMDBKey: checking the key: %v", err)
	httpx.WriteError(w, http.StatusBadGateway, "Couldn't reach TMDB to check this key. Nothing was saved; try again in a moment.")
}

// storeTMDBKey seals key for the signed-in account and saves it.
func (s *Server) storeTMDBKey(ctx context.Context, key string) (tmdbKeyStatus, error) {
	sub, _ := nuvioUserIDFrom(ctx) // guaranteed by requireNuvioAuth
	stored, err := s.keys.Seal(tmdbkey.Provider, sub, key)
	if err != nil {
		return tmdbKeyStatus{}, err
	}
	if err := s.vault.SetAccountKey(ctx, sub, tmdbkey.Provider, stored); err != nil {
		return tmdbKeyStatus{}, err
	}
	return tmdbKeyStatus{Set: true, Last4: stored.Last4}, nil
}

// deleteTMDBKey answers DELETE /api/account/tmdb-key: the signed-in account's
// key is removed, if it had one.
func (s *Server) deleteTMDBKey(w http.ResponseWriter, r *http.Request) {
	sub, _ := nuvioUserIDFrom(r.Context()) // guaranteed by requireNuvioAuth
	if err := s.vault.DeleteAccountKey(r.Context(), sub, tmdbkey.Provider); err != nil {
		serverError(w, "deleteTMDBKey", err, "failed to remove your TMDB key")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
