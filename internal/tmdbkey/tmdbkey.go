// Package tmdbkey holds the TMDB keys accounts bring on a server in
// per-account key mode: it seals a key for the vault under the server's
// secret, and gives each request a provider.KeySource that opens the key of
// the account the request is for, only if TMDB is reached.
//
// A key is never returned, never logged, and only ever sent to TMDB.
package tmdbkey

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

// SecretSize is the length of the server's secret, UNO_SECRET decoded: an
// AES-256 key.
const SecretSize = 32

// Box seals and opens TMDB keys with AES-256-GCM under the server's secret.
// Each sealed key is bound to its account, so a sealed key moved to another
// account's row doesn't open.
type Box struct {
	aead cipher.AEAD
}

// NewBox builds a Box from the server's secret, SecretSize bytes.
func NewBox(secret []byte) (*Box, error) {
	if len(secret) != SecretSize {
		return nil, fmt.Errorf("tmdbkey: the secret is %d bytes; want %d", len(secret), SecretSize)
	}
	block, err := aes.NewCipher(secret)
	if err != nil {
		return nil, fmt.Errorf("tmdbkey: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("tmdbkey: %w", err)
	}
	return &Box{aead: aead}, nil
}

// boundTo is the additional data that binds a sealed key to account.
func boundTo(account string) []byte {
	return []byte("uno-tmdb-key/1\x00" + account)
}

// Seal is key sealed for account: a random nonce, then the ciphertext.
func (b *Box) Seal(account, key string) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("tmdbkey: drawing a nonce: %w", err)
	}
	return b.aead.Seal(nonce, nonce, []byte(key), boundTo(account)), nil
}

// Open is the key sealed holds for account. It fails for a key sealed under
// another secret or for another account, and for anything tampered with.
func (b *Box) Open(account string, sealed []byte) (string, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n {
		return "", errors.New("tmdbkey: the sealed key is too short")
	}
	key, err := b.aead.Open(nil, sealed[:n], sealed[n:], boundTo(account))
	if err != nil {
		return "", fmt.Errorf("tmdbkey: opening the key: %w", err)
	}
	return string(key), nil
}

// ErrNotAPIKey and ErrReadAccessToken are a key that isn't the shape of a
// TMDB API key; the second is TMDB's Read Access Token, which sits beside it
// on the same TMDB page and is easy to paste instead.
var (
	ErrNotAPIKey       = errors.New("tmdbkey: not a TMDB API key")
	ErrReadAccessToken = errors.New("tmdbkey: a TMDB Read Access Token, not an API key")
)

// apiKeyShape is a TMDB API key: 32 hexadecimal characters.
var apiKeyShape = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

// Clean is key as pasted, trimmed, when it is the shape of a TMDB API key.
// A Read Access Token (a JWT, so "eyJ…") is ErrReadAccessToken, anything else
// ErrNotAPIKey.
func Clean(key string) (string, error) {
	key = strings.TrimSpace(key)
	switch {
	case apiKeyShape.MatchString(key):
		return key, nil
	case strings.HasPrefix(key, "eyJ"):
		return "", ErrReadAccessToken
	}
	return "", ErrNotAPIKey
}

// Keys gives requests the TMDB key of the account they are for. A nil *Keys
// is a server with one shared key: every source it gives is nil, which
// provider.WithKeySource ignores, so callers never ask which mode they run in.
type Keys struct {
	box   *Box
	vault *vault.DB
}

// New builds Keys over box and the vault that stores the sealed keys.
func New(box *Box, v *vault.DB) *Keys {
	return &Keys{box: box, vault: v}
}

// Seal is key sealed for account, as the vault stores it, with its last four
// characters.
func (k *Keys) Seal(account, key string) (vault.AccountKey, error) {
	sealed, err := k.box.Seal(account, key)
	if err != nil {
		return vault.AccountKey{}, err
	}
	return vault.AccountKey{Sealed: sealed, Last4: key[max(len(key)-4, 0):]}, nil
}

// ForAccount is the key source of a request signed in as account.
func (k *Keys) ForAccount(ctx context.Context, account string) provider.KeySource {
	if k == nil {
		return nil
	}
	ctx = context.WithoutCancel(ctx) // a shared page fetch outlives its request
	return func() (string, error) {
		stored, err := k.vault.AccountKey(ctx, account)
		if errors.Is(err, vault.ErrNoAccountKey) {
			return "", provider.ErrNoKey
		}
		if err != nil {
			return "", err
		}
		return k.open(account, stored.Sealed)
	}
}

// ForToken is the key source of a request for the profile whose addon token
// it is: its owner's key.
func (k *Keys) ForToken(ctx context.Context, token string) provider.KeySource {
	if k == nil {
		return nil
	}
	ctx = context.WithoutCancel(ctx)
	return func() (string, error) {
		account, sealed, err := k.vault.AccountKeyByToken(ctx, token)
		if err != nil {
			return "", err
		}
		return k.open(account, sealed)
	}
}

// Sealed is the key source of a request for account whose sealed key is
// already read, nil when it has none.
func (k *Keys) Sealed(account string, sealed []byte) provider.KeySource {
	if k == nil {
		return nil
	}
	return func() (string, error) { return k.open(account, sealed) }
}

// open is account's key from sealed: ErrNoKey when there is none, and a key
// that doesn't open (the server's secret changed) is a rejected one, which its
// owner fixes the same way, by entering it again.
func (k *Keys) open(account string, sealed []byte) (string, error) {
	if sealed == nil {
		return "", provider.ErrNoKey
	}
	key, err := k.box.Open(account, sealed)
	if err != nil {
		return "", fmt.Errorf("%w: %w", provider.ErrKeyRejected, err)
	}
	return key, nil
}
