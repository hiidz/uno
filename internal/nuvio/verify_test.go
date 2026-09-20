package nuvio

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// keyForScalar returns the P-256 key pair for a fixed private scalar, so the
// fixtures below are the same key material on every run with no random source
// involved.
func keyForScalar(t *testing.T, scalar uint32) *ecdsa.PrivateKey {
	t.Helper()
	raw := make([]byte, p256CoordBytes)
	binary.BigEndian.PutUint32(raw[p256CoordBytes-4:], scalar)
	priv, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), raw)
	if err != nil {
		t.Fatalf("parsing fixed private scalar %d: %v", scalar, err)
	}
	return priv
}

// uncompressed is pub's 0x04 ‖ X ‖ Y encoding, the form a JWK's x and y are
// the two halves of.
func uncompressed(t *testing.T, pub *ecdsa.PublicKey) []byte {
	t.Helper()
	point, err := pub.Bytes()
	if err != nil {
		t.Fatalf("encoding public key: %v", err)
	}
	if len(point) != 1+2*p256CoordBytes {
		t.Fatalf("public key encoded to %d bytes, want %d", len(point), 1+2*p256CoordBytes)
	}
	return point
}

// jwkCoords is pub's base64url-encoded JWK x and y members.
func jwkCoords(t *testing.T, pub *ecdsa.PublicKey) (x, y string) {
	t.Helper()
	point := uncompressed(t, pub)
	return base64.RawURLEncoding.EncodeToString(point[1 : 1+p256CoordBytes]),
		base64.RawURLEncoding.EncodeToString(point[1+p256CoordBytes:])
}

type testJWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// jwksServer serves keys at the path fetchKeys asks for, and 404s anything
// else so a changed path fails the test rather than passing silently.
func jwksServer(t *testing.T, keys []testJWK) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/.well-known/jwks.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			Keys []testJWK `json:"keys"`
		}{keys}); err != nil {
			t.Errorf("encoding JWKS: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestVerifyES256TokenAgainstJWKS is the whole auth path: a JWKS published in
// Nuvio's shape, a token signed with the matching private key, and the claims
// Verify hands back.
func TestVerifyES256TokenAgainstJWKS(t *testing.T) {
	priv := keyForScalar(t, 7)
	x, y := jwkCoords(t, &priv.PublicKey)
	srv := jwksServer(t, []testJWK{{Kid: "kid-1", Kty: "EC", Crv: "P-256", X: x, Y: y}})

	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": srv.URL + "/auth/v1",
		"sub": "nuvio-user-1",
		"exp": jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	token.Header["kid"] = "kid-1"
	signed, err := token.SignedString(priv)
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}

	v := NewVerifier(srv.URL)
	claims, err := v.Verify(context.Background(), signed)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Sub != "nuvio-user-1" {
		t.Errorf("Sub = %q, want %q", claims.Sub, "nuvio-user-1")
	}

	cached, ok := v.keys["kid-1"]
	if !ok {
		t.Fatal("kid-1 was not cached")
	}
	if !cached.Equal(&priv.PublicKey) {
		t.Error("cached key is not the key the JWKS published")
	}
}

// TestFetchKeysSkipsUnusableKeys pins the skip-and-continue contract: a key
// this package can't use is dropped, the fetch still succeeds, and the usable
// key in the same set is cached.
func TestFetchKeysSkipsUnusableKeys(t *testing.T) {
	priv := keyForScalar(t, 11)
	x, y := jwkCoords(t, &priv.PublicKey)
	zeros := base64.RawURLEncoding.EncodeToString(make([]byte, p256CoordBytes))
	tooWide := base64.RawURLEncoding.EncodeToString(make([]byte, p256CoordBytes+1))

	srv := jwksServer(t, []testJWK{
		{Kid: "rsa", Kty: "RSA", Crv: "P-256", X: x, Y: y},
		{Kid: "p384", Kty: "EC", Crv: "P-384", X: x, Y: y},
		{Kid: "not-base64", Kty: "EC", Crv: "P-256", X: "!!!", Y: y},
		{Kid: "too-wide", Kty: "EC", Crv: "P-256", X: tooWide, Y: y},
		{Kid: "off-curve", Kty: "EC", Crv: "P-256", X: zeros, Y: zeros},
		{Kid: "empty", Kty: "EC", Crv: "P-256"},
		{Kid: "good", Kty: "EC", Crv: "P-256", X: x, Y: y},
	})

	v := NewVerifier(srv.URL)
	if err := v.fetchKeys(context.Background()); err != nil {
		t.Fatalf("fetchKeys: %v", err)
	}

	if len(v.keys) != 1 {
		t.Fatalf("cached %d keys, want only the usable one: %v", len(v.keys), v.keys)
	}
	cached, ok := v.keys["good"]
	if !ok {
		t.Fatal("the usable key was not cached")
	}
	if !cached.Equal(&priv.PublicKey) {
		t.Error("cached key is not the key the JWKS published")
	}
}

// TestParseP256JWKPadsStrippedLeadingZero covers a coordinate presented 31
// bytes wide because its leading zero byte was dropped.
func TestParseP256JWKPadsStrippedLeadingZero(t *testing.T) {
	var priv *ecdsa.PrivateKey
	var point []byte
	for scalar := uint32(1); scalar <= 4096; scalar++ {
		candidate := keyForScalar(t, scalar)
		encoded := uncompressed(t, &candidate.PublicKey)
		if encoded[1] == 0 {
			priv, point = candidate, encoded
			break
		}
	}
	if priv == nil {
		t.Fatal("no P-256 key with a leading zero byte in X among the first 4096 scalars")
	}

	stripped := base64.RawURLEncoding.EncodeToString(point[2 : 1+p256CoordBytes])
	y := base64.RawURLEncoding.EncodeToString(point[1+p256CoordBytes:])

	got, err := parseP256JWK(stripped, y)
	if err != nil {
		t.Fatalf("parseP256JWK with a 31-byte x: %v", err)
	}
	if !got.Equal(&priv.PublicKey) {
		t.Error("left-padding the stripped coordinate did not reproduce the key")
	}
}

func TestParseP256JWKRejects(t *testing.T) {
	priv := keyForScalar(t, 13)
	x, y := jwkCoords(t, &priv.PublicKey)
	zeros := base64.RawURLEncoding.EncodeToString(make([]byte, p256CoordBytes))
	tooWide := base64.RawURLEncoding.EncodeToString(make([]byte, p256CoordBytes+1))

	tests := map[string]struct{ x, y string }{
		"x not base64":       {"!!!", y},
		"y not base64":       {x, "!!!"},
		"x wider than 32":    {tooWide, y},
		"y wider than 32":    {x, tooWide},
		"point not on curve": {zeros, zeros},
		"y does not match x": {x, zeros},
		"both empty":         {"", ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseP256JWK(tc.x, tc.y); err == nil {
				t.Error("want an error, got none")
			}
		})
	}
}
