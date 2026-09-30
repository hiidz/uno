package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
)

// The TMDB key modes TMDB_KEY_MODE takes.
const (
	keyModeShared     = "shared"
	keyModePerAccount = "per-account"
)

// secretSize is UNO_SECRET's length once decoded: an AES-256 key.
const secretSize = 32

// tmdbKeys is how the server reaches TMDB: with apiKey, shared by every
// account, or, with perAccount set, with each account's own key, sealed
// under secret.
type tmdbKeys struct {
	apiKey     string
	perAccount bool
	secret     []byte
}

// loadTMDB reads TMDB_KEY_MODE and the variable its mode needs: TMDB_API_KEY
// in shared mode, UNO_SECRET in per-account mode.
func loadTMDB() (tmdbKeys, error) {
	return tmdbKeysFor(getEnv("TMDB_KEY_MODE", keyModeShared), os.Getenv("TMDB_API_KEY"), os.Getenv("UNO_SECRET"))
}

// tmdbKeysFor is the mode mode asks for, given the two variables. It refuses
// an unknown mode, shared mode without a key, and per-account mode with a
// shared key (which would read as a fallback that isn't there) or without a
// usable secret. UNO_SECRET is ignored in shared mode, so it can stay set
// while a server that has stored keys runs shared for a while.
func tmdbKeysFor(mode, apiKey, secret string) (tmdbKeys, error) {
	switch mode {
	case keyModeShared:
		if apiKey == "" {
			return tmdbKeys{}, errors.New("missing required environment variable TMDB_API_KEY: TMDB_KEY_MODE is shared (the default), where every account uses it")
		}
		return tmdbKeys{apiKey: apiKey}, nil
	case keyModePerAccount:
		return perAccountKeys(apiKey, secret)
	}
	return tmdbKeys{}, fmt.Errorf("TMDB_KEY_MODE is %q; want %q or %q", mode, keyModeShared, keyModePerAccount)
}

// perAccountKeys is per-account mode, with secret decoded.
func perAccountKeys(apiKey, secret string) (tmdbKeys, error) {
	if apiKey != "" {
		return tmdbKeys{}, errors.New("TMDB_API_KEY is set but TMDB_KEY_MODE is per-account, where each account brings its own key; unset TMDB_API_KEY")
	}
	key, err := base64.StdEncoding.DecodeString(secret)
	if err != nil || len(key) != secretSize {
		return tmdbKeys{}, fmt.Errorf("UNO_SECRET must be %d random bytes, base64-encoded (openssl rand -base64 32), when TMDB_KEY_MODE is per-account", secretSize)
	}
	return tmdbKeys{perAccount: true, secret: key}, nil
}
