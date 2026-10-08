package nuvio

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// The errors Verify returns: ErrInvalidToken for a token that fails any check
// (signature, claims, expiry), the caller's fault, and ErrJWKSUnavailable when
// Nuvio's signing keys can't be fetched, upstream's.
var (
	ErrInvalidToken    = errors.New("nuvio: invalid or expired token")
	ErrJWKSUnavailable = errors.New("nuvio: could not fetch signing keys")
)

// minRefetchInterval bounds how often Verify will hit the JWKS endpoint
// again for a kid it doesn't recognize, whether the last fetch succeeded or
// failed, so a burst of tokens carrying a bogus kid, or a JWKS outage, can't
// trigger a fetch per request.
const minRefetchInterval = 5 * time.Second

// p256CoordBytes is the fixed width of a P-256 coordinate, and so half of an
// uncompressed point's payload.
const p256CoordBytes = 32

// Verifier turns a Nuvio-issued bearer token into trusted claims by
// checking its signature against Nuvio's published JWKS. It holds no
// credential of its own — JWKS is a public endpoint.
type Verifier struct {
	baseURL string
	http    *http.Client
	mu      sync.RWMutex
	keys    map[string]*ecdsa.PublicKey // kid -> public key

	// fetchMu holds one JWKS fetch at a time: callers that miss the cache
	// together wait for the one fetch and share its outcome. It guards
	// fetched, the last fetch's start, and fetchErr, its error (nil on
	// success).
	fetchMu  sync.Mutex
	fetched  time.Time
	fetchErr error
}

// NewVerifier builds a Verifier that fetches and caches signing keys from
// baseURL's JWKS endpoint.
func NewVerifier(baseURL string) *Verifier {
	return &Verifier{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
		keys:    make(map[string]*ecdsa.PublicKey),
	}
}

// Verify parses tokenString as a JWT, checks its signature against a
// matching cached (or freshly fetched) JWKS key, and returns its claims.
func (v *Verifier) Verify(ctx context.Context, tokenString string) (Claims, error) {
	key, err := v.resolveKey(ctx, tokenString)
	if err != nil {
		return Claims{}, err
	}

	claims := jwt.MapClaims{}
	_, err = jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		return key, nil
	}, jwt.WithValidMethods([]string{"ES256"}), jwt.WithAudience(signedInAudience))
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if problem := v.claimsProblem(claims); problem != "" {
		return Claims{}, fmt.Errorf("%w: %s", ErrInvalidToken, problem)
	}

	sub, _ := claims.GetSubject()
	// Supabase writes the account's email address into every token it signs;
	// one without it reads as "".
	email, _ := claims["email"].(string)

	return Claims{Sub: sub, Email: email}, nil
}

// signedInAudience is the aud Nuvio's auth server gives the tokens of
// accounts that have signed in (GoTrue's JWT_AUD).
const signedInAudience = "authenticated"

// claimsProblem is why claims, already checked for signature, expiry and
// audience, are no sign-in of a Nuvio account, or "" when nothing is: an
// issuer other than v's Nuvio, no subject or expiry, or an anonymous
// sign-in. Nuvio lets anyone sign in anonymously, with no email or password;
// Uno's own login never does, so only a script would bring such a token, and
// each would be a fresh account.
func (v *Verifier) claimsProblem(claims jwt.MapClaims) string {
	iss, _ := claims.GetIssuer()
	sub, _ := claims.GetSubject()
	exp, _ := claims.GetExpirationTime()
	anonymous, _ := claims["is_anonymous"].(bool)
	switch {
	case iss != v.baseURL+"/auth/v1":
		return fmt.Sprintf("unexpected issuer %q", iss)
	case sub == "":
		return "missing subject"
	case exp == nil:
		return "missing expiry"
	case anonymous:
		return "an anonymous sign-in"
	}
	return ""
}

// resolveKey extracts kid from tokenString's header and returns the
// matching public key, refetching the JWKS (refreshKeys) when the kid isn't
// cached.
func (v *Verifier) resolveKey(ctx context.Context, tokenString string) (*ecdsa.PublicKey, error) {
	kid, err := extractKid(tokenString)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if key, ok := v.cachedKey(kid); ok {
		return key, nil
	}
	if err := v.refreshKeys(ctx); err != nil {
		return nil, err
	}
	if key, ok := v.cachedKey(kid); ok {
		return key, nil
	}
	return nil, ErrInvalidToken
}

// cachedKey is the cached public key for kid, if there is one.
func (v *Verifier) cachedKey(kid string) (*ecdsa.PublicKey, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	key, ok := v.keys[kid]
	return key, ok
}

// refreshKeys fetches the JWKS, unless a fetch started within
// minRefetchInterval, in which case it answers with that fetch's error.
// A caller that arrives while a fetch is in flight waits for it, so
// concurrent cache misses make one request between them. The fetch runs
// detached from ctx's cancellation: one caller giving up would otherwise fail
// it for every caller sharing it, for the whole interval.
func (v *Verifier) refreshKeys(ctx context.Context) error {
	v.fetchMu.Lock()
	defer v.fetchMu.Unlock()
	if time.Since(v.fetched) < minRefetchInterval {
		return v.fetchErr
	}
	v.fetched = time.Now()
	v.fetchErr = v.fetchKeys(context.WithoutCancel(ctx))
	return v.fetchErr
}

// extractKid reads the kid header without verifying the signature — we
// need it to know *which* key to verify against in the first place.
func extractKid(tokenString string) (string, error) {
	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", err
	}
	kid, ok := token.Header["kid"].(string)
	if !ok || kid == "" {
		return "", fmt.Errorf("token header missing kid")
	}
	return kid, nil
}

func (v *Verifier) fetchKeys(ctx context.Context) error {
	url := v.baseURL + "/auth/v1/.well-known/jwks.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrJWKSUnavailable, err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := v.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrJWKSUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %s", ErrJWKSUnavailable, resp.Status)
	}

	var body struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&body); err != nil {
		return fmt.Errorf("%w: %w", ErrJWKSUnavailable, err)
	}

	keys := usableKeys(body.Keys)
	v.mu.Lock()
	v.keys = keys
	v.mu.Unlock()

	return nil
}

// jwk is one key of a JWKS, the members of it this package reads.
type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// usableKeys is the P-256 keys among set, by kid. One unusable key doesn't
// sink the whole JWKS: it is logged and skipped, and the rest of the set
// still verifies the tokens signed with it.
func usableKeys(set []jwk) map[string]*ecdsa.PublicKey {
	keys := make(map[string]*ecdsa.PublicKey, len(set))
	for _, k := range set {
		if k.Kty != "EC" || k.Crv != "P-256" {
			continue // not a key shape we know how to handle
		}
		key, err := parseP256JWK(k.X, k.Y)
		if err != nil {
			log.Printf("nuvio: skipping JWKS key %q: %v", k.Kid, err)
			continue
		}
		keys[k.Kid] = key
	}
	return keys
}

// parseP256JWK builds a P-256 public key from a JWK's base64url-encoded "x"
// and "y" coordinates, rejecting a point that isn't on the curve.
//
// Each coordinate is left-padded to p256CoordBytes: a conforming issuer sends
// exactly that many bytes, but a leading zero byte can legitimately be
// dropped, and one wider than that is not a P-256 coordinate at all.
func parseP256JWK(x, y string) (*ecdsa.PublicKey, error) {
	xBytes, err := base64.RawURLEncoding.DecodeString(x)
	if err != nil {
		return nil, fmt.Errorf("decoding x: %w", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(y)
	if err != nil {
		return nil, fmt.Errorf("decoding y: %w", err)
	}
	if len(xBytes) > p256CoordBytes || len(yBytes) > p256CoordBytes {
		return nil, fmt.Errorf("coordinate wider than %d bytes (x=%d, y=%d)", p256CoordBytes, len(xBytes), len(yBytes))
	}

	// 0x04 ‖ X ‖ Y: the uncompressed point encoding ParseUncompressedPublicKey
	// expects, with each coordinate right-aligned in its own 32-byte half.
	point := make([]byte, 1+2*p256CoordBytes)
	point[0] = 4
	copy(point[1+p256CoordBytes-len(xBytes):], xBytes)
	copy(point[1+2*p256CoordBytes-len(yBytes):], yBytes)

	return ecdsa.ParseUncompressedPublicKey(elliptic.P256(), point)
}
