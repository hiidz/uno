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

var (
	ErrInvalidToken    = errors.New("nuvio: invalid or expired token")
	ErrJWKSUnavailable = errors.New("nuvio: could not fetch signing keys")
)

// minRefetchInterval bounds how often Verify will hit the JWKS endpoint
// again for a kid it doesn't recognize, so a burst of tokens carrying a
// bogus kid can't trigger a fetch per request.
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
	fetched time.Time
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
	}, jwt.WithValidMethods([]string{"ES256"}))
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	iss, _ := claims.GetIssuer()
	wantIss := v.baseURL + "/auth/v1"
	if iss != wantIss {
		return Claims{}, fmt.Errorf("%w: unexpected issuer %q", ErrInvalidToken, iss)
	}

	sub, err := claims.GetSubject()
	if err != nil || sub == "" {
		return Claims{}, fmt.Errorf("%w: missing subject", ErrInvalidToken)
	}

	expTime, err := claims.GetExpirationTime()
	if err != nil || expTime == nil {
		return Claims{}, fmt.Errorf("%w: missing expiry", ErrInvalidToken)
	}

	// Supabase writes the account's email address into every token it signs;
	// one without it reads as "".
	email, _ := claims["email"].(string)

	return Claims{Sub: sub, Email: email, Exp: expTime.Time}, nil
}

// resolveKey extracts kid from tokenString's header and returns the
// matching public key, refetching the JWKS at most once per
// minRefetchInterval when the kid isn't cached.
func (v *Verifier) resolveKey(ctx context.Context, tokenString string) (*ecdsa.PublicKey, error) {
	kid, err := extractKid(tokenString)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	v.mu.RLock()
	key, ok := v.keys[kid]
	lastFetch := v.fetched
	v.mu.RUnlock()
	if ok {
		return key, nil
	}

	if time.Since(lastFetch) < minRefetchInterval {
		// We just refetched and this kid still wasn't in it — don't
		// refetch again for a repeat offender within the window.
		return nil, ErrInvalidToken
	}

	if err := v.fetchKeys(ctx); err != nil {
		return nil, err
	}

	v.mu.RLock()
	key, ok = v.keys[kid]
	v.mu.RUnlock()
	if !ok {
		return nil, ErrInvalidToken
	}
	return key, nil
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

	resp, err := v.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrJWKSUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %s", ErrJWKSUnavailable, resp.Status)
	}

	var body struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&body); err != nil {
		return fmt.Errorf("%w: %w", ErrJWKSUnavailable, err)
	}

	keys := make(map[string]*ecdsa.PublicKey, len(body.Keys))
	for _, k := range body.Keys {
		if k.Kty != "EC" || k.Crv != "P-256" {
			continue // not a key shape we know how to handle
		}
		key, err := parseP256JWK(k.X, k.Y)
		if err != nil {
			// One unusable key doesn't sink the whole JWKS: the rest of the
			// set still verifies the tokens signed with it.
			log.Printf("nuvio: skipping JWKS key %q: %v", k.Kid, err)
			continue
		}
		keys[k.Kid] = key
	}

	v.mu.Lock()
	v.keys = keys
	v.fetched = time.Now()
	v.mu.Unlock()

	return nil
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
