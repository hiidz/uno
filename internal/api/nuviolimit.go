package api

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/hiidz/uno/internal/httpx"
)

// nuvioCallBurst and nuvioCallEvery pace each account's requests to the
// routes that call Nuvio (limitNuvioCalls): nuvioCallBurst at once, then one
// more every nuvioCallEvery. Nuvio counts every client of every app against
// one shared budget, so no account may spend much of it through Uno; a person
// using the builder stays well under this.
const (
	nuvioCallBurst = 10
	nuvioCallEvery = 3 * time.Second
)

// accountLimiterSweep is how often accountLimiter drops the buckets of
// accounts that have gone quiet.
const accountLimiterSweep = time.Minute

// accountLimiter is a token bucket per account: burst requests held, one more
// every refill. It refuses instead of waiting.
type accountLimiter struct {
	mu        sync.Mutex
	burst     float64
	refill    time.Duration
	byAccount map[string]*accountBucket
	swept     time.Time
	now       func() time.Time
}

// accountBucket is one account's tokens as of last.
type accountBucket struct {
	tokens float64
	last   time.Time
}

func newAccountLimiter(burst int, refill time.Duration) *accountLimiter {
	return &accountLimiter{burst: float64(burst), refill: refill, byAccount: make(map[string]*accountBucket), now: time.Now}
}

// allow spends one of account's tokens and returns zero, or returns how long
// until it has one, spending nothing.
func (l *accountLimiter) allow(account string) time.Duration {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now)
	b, ok := l.byAccount[account]
	if !ok {
		b = &accountBucket{tokens: l.burst, last: now}
		l.byAccount[account] = b
	}
	b.tokens = l.tokensAt(b, now)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return 0
	}
	return time.Duration((1 - b.tokens) * float64(l.refill))
}

// tokensAt is b's tokens refilled up to now, at most burst.
func (l *accountLimiter) tokensAt(b *accountBucket, now time.Time) float64 {
	if !now.After(b.last) {
		return b.tokens
	}
	return min(l.burst, b.tokens+float64(now.Sub(b.last))/float64(l.refill))
}

// sweep drops, at most once every accountLimiterSweep, each bucket that has
// refilled: a fresh one would behave the same. The caller holds l.mu.
func (l *accountLimiter) sweep(now time.Time) {
	if now.Sub(l.swept) < accountLimiterSweep {
		return
	}
	l.swept = now
	for account, b := range l.byAccount {
		if l.tokensAt(b, now) >= l.burst {
			delete(l.byAccount, account)
		}
	}
}

// limitNuvioCalls answers a request from an account over its nuvioCalls
// budget with a 429 and the whole seconds to wait as Retry-After, before next
// runs and so before any call to Nuvio. Must be chained after requireNuvioAuth.
func (s *Server) limitNuvioCalls(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sub, _ := nuvioUserIDFrom(r.Context()) // guaranteed by requireNuvioAuth
		if wait := s.nuvioCalls.allow(sub); wait > 0 {
			writeTooManyRequests(w, wait, "too many requests")
			return
		}
		next(w, r)
	}
}

// writeTooManyRequests answers a 429 with msg, and wait as Retry-After in
// whole seconds, rounded up so a client that waits that long is let through.
func writeTooManyRequests(w http.ResponseWriter, wait time.Duration, msg string) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(wait)))
	httpx.WriteError(w, http.StatusTooManyRequests, msg)
}

// retryAfterSeconds is wait in whole seconds, rounded up, at least 1.
func retryAfterSeconds(wait time.Duration) int {
	return max(1, int(math.Ceil(wait.Seconds())))
}
