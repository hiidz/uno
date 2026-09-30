package provider

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"
)

// ErrNoKey is a TMDB call with no key to make it with: the account it is for
// has saved none, on a server that asks each account for its own.
var ErrNoKey = errors.New("provider: no TMDB key for this account")

// ErrKeyRejected is a TMDB 401 on an account's own key: TMDB doesn't accept
// it. A 401 on the server's shared key is the operator's to fix, and stays an
// ordinary upstream failure.
var ErrKeyRejected = errors.New("provider: TMDB rejected the account's key")

// IsKeyError reports whether err is a problem with an account's own key,
// which only its owner can fix.
func IsKeyError(err error) bool {
	return errors.Is(err, ErrNoKey) || errors.Is(err, ErrKeyRejected)
}

// KeySource yields the TMDB key of the account a request is for: the key, or
// ErrNoKey when the account has saved none, or why it couldn't be read.
type KeySource func() (string, error)

type keySourceKey struct{}

// WithKeySource is ctx carrying src, so every TMDB call made with it uses the
// account's own key rather than the client's shared one. src runs at most
// once, when the first of those calls goes out, so a request that reaches
// TMDB for nothing never reads a key. A nil src leaves ctx as it is.
func WithKeySource(ctx context.Context, src KeySource) context.Context {
	if src == nil {
		return ctx
	}
	return context.WithValue(ctx, keySourceKey{}, KeySource(sync.OnceValues(src)))
}

// apiKey is the key one TMDB call goes out with; own when it is an account's
// rather than the client's shared one.
type apiKey struct {
	value string
	own   bool
}

// keyFor is the key a call made with ctx uses: the account's, from ctx's
// KeySource, else the client's shared key. A client with no shared key serves
// accounts that bring their own, so a call without one is ErrNoKey.
func (c *TMDBClient) keyFor(ctx context.Context) (apiKey, error) {
	if src, ok := ctx.Value(keySourceKey{}).(KeySource); ok {
		key, err := src()
		return apiKey{value: key, own: true}, err
	}
	if c.apiKey == "" {
		return apiKey{}, ErrNoKey
	}
	return apiKey{value: c.apiKey}, nil
}

// CheckKey asks TMDB, with one call, whether key is an API key it accepts. A
// key it refuses is ErrKeyRejected.
func (c *TMDBClient) CheckKey(ctx context.Context, key string) error {
	ctx = WithKeySource(ctx, func() (string, error) { return key, nil })
	var out struct {
		Success bool `json:"success"`
	}
	return c.get(ctx, "/authentication", nil, &out)
}

// withoutURL is err without the request URL a failed request's *url.Error
// carries, since that URL holds the api_key: the error is logged, and a key
// never is.
func withoutURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("%s: %w", urlErr.Op, urlErr.Err)
	}
	return err
}

// perKeyRequestsPerSecond and perKeyRequestBurst pace the calls made with one
// account's own key, under the process-wide limiter, so one account can't
// take the whole of the server's TMDB budget.
const (
	perKeyRequestsPerSecond = 20
	perKeyRequestBurst      = 40
)

// keyLimiterSweep is how often keyLimiters drops the limiters of keys that
// have gone quiet.
const keyLimiterSweep = time.Minute

// keyLimiters holds one limiter per account key, keyed by the key's SHA-256 so
// the map holds no key.
type keyLimiters struct {
	mu    sync.Mutex
	rate  float64
	burst float64
	byKey map[[sha256.Size]byte]*limiter
	swept time.Time
}

func newKeyLimiters(rate, burst float64) *keyLimiters {
	return &keyLimiters{rate: rate, burst: burst, byKey: make(map[[sha256.Size]byte]*limiter)}
}

// wait blocks until key's limiter lets one request out, or ctx ends.
func (k *keyLimiters) wait(ctx context.Context, key string) error {
	return waitFor(ctx, func(now time.Time) time.Duration { return k.reserve(key, now) })
}

// reserve is limiter.reserve on key's limiter, made on first use. The token
// is taken under k.mu, the lock sweep holds, so no limiter is swept between
// being found and being spent from.
func (k *keyLimiters) reserve(key string, now time.Time) time.Duration {
	id := keyID(key)
	k.mu.Lock()
	defer k.mu.Unlock()
	k.sweep(now)
	l, ok := k.byKey[id]
	if !ok {
		l = newLimiter(k.rate, k.burst)
		k.byKey[id] = l
	}
	return l.reserve(now)
}

// keyID is what keyLimiters files key under: its SHA-256.
func keyID(key string) [sha256.Size]byte {
	return sha256.Sum256([]byte(key))
}

// sweep drops, at most once every keyLimiterSweep, each limiter whose bucket
// has refilled: a fresh one would behave the same. The caller holds k.mu.
func (k *keyLimiters) sweep(now time.Time) {
	if now.Sub(k.swept) < keyLimiterSweep {
		return
	}
	k.swept = now
	for id, l := range k.byKey {
		if l.full(now) {
			delete(k.byKey, id)
		}
	}
}

// waitTurn waits for key's own limiter when it is an account's, then for the
// process-wide one every call shares.
func (c *TMDBClient) waitTurn(ctx context.Context, key apiKey) error {
	if key.own {
		if err := c.keyLimiters.wait(ctx, key.value); err != nil {
			return err
		}
	}
	return c.limiter.wait(ctx)
}
