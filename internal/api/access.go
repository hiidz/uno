package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/hiidz/uno/internal/nuvio"
)

// Access says which Nuvio accounts may use the builder API: every verified
// account, or, with Allowlist set, only those whose token carries an email
// address in Emails (lower-cased), and the dev auth bypass's fake account,
// which has none, when DevBypass is set. The zero Access admits every
// account.
type Access struct {
	Allowlist bool
	Emails    []string
	DevBypass bool
}

// WithDevBypass is a, admitting the dev auth bypass's fake account as well,
// by its id, so an allowlist doesn't shut out local development. cmd/uno
// applies it only when the bypass is configured.
func (a Access) WithDevBypass() Access {
	a.DevBypass = true
	return a
}

// admission is an allowlist ready to check: its emails as a set, and
// whether it admits the dev bypass's account.
type admission struct {
	emails    map[string]bool
	devBypass bool
}

// admission is a as an admission, or nil when a admits every account.
func (a Access) admission() *admission {
	if !a.Allowlist {
		return nil
	}
	emails := make(map[string]bool, len(a.Emails))
	for _, email := range a.Emails {
		emails[email] = true
	}
	return &admission{emails: emails, devBypass: a.DevBypass}
}

// admits reports whether the account claims names may use the builder API:
// any account when no allowlist is set, else as the allowlist admits it.
func (s *Server) admits(claims nuvio.Claims) bool {
	return s.admission == nil || s.admission.admits(claims)
}

// admits reports whether a admits the account claims names: the dev
// bypass's by its id, and otherwise one whose email, lower-cased, is listed.
// A token without an email matches no entry.
func (a *admission) admits(claims nuvio.Claims) bool {
	return a.devBypass && claims.Sub == DevBypassSub || a.emails[strings.ToLower(claims.Email)]
}

// authRefusal is why requireNuvioAuth turned a request away: the status and
// the words it answers with.
type authRefusal struct {
	status int
	msg    string
}

// errNotAdmitted answers an account the access policy doesn't admit. It is a
// 403, never a 401: the SPA answers a 401 by refreshing its token and trying
// again, which a refused account would do forever.
var errNotAdmitted = &authRefusal{http.StatusForbidden, "this Nuvio account can't use this Uno server"}

// authenticate verifies r's bearer token and checks its account against the
// access policy, returning the token and its claims, or why r is refused.
func (s *Server) authenticate(r *http.Request) (string, nuvio.Claims, *authRefusal) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return "", nuvio.Claims{}, &authRefusal{http.StatusUnauthorized, "missing or malformed Authorization header"}
	}
	claims, err := s.verifier.Verify(r.Context(), token)
	if err != nil {
		return "", nuvio.Claims{}, verifyRefusal(err)
	}
	if !s.admits(claims) {
		return "", nuvio.Claims{}, errNotAdmitted
	}
	return token, claims, nil
}

// verifyRefusal is the answer to a token that failed verification: an
// unreachable JWKS is upstream's fault (502), anything else the token's (401).
func verifyRefusal(err error) *authRefusal {
	switch {
	case errors.Is(err, nuvio.ErrInvalidToken):
		return &authRefusal{http.StatusUnauthorized, "invalid or expired token"}
	case errors.Is(err, nuvio.ErrJWKSUnavailable):
		return &authRefusal{http.StatusBadGateway, "auth service unavailable"}
	}
	return &authRefusal{http.StatusUnauthorized, "authentication failed"}
}
