package provider

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// tmdbRequestsPerSecond and tmdbRequestBurst are Uno's own ceiling on the
// TMDB API calls one process makes, whoever they are for: the builder's
// lookups and previews and every catalog page the addon serves.
const (
	tmdbRequestsPerSecond = 40
	tmdbRequestBurst      = 40
)

// defaultRetryAfter is how long a 429 without a usable Retry-After pauses
// every TMDB call; maxRetryAfter caps what one may ask for. The cap sits well
// under catalogPageFetchTimeout, so a page fetch waiting out a pause still
// has time to finish; a TMDB that asks for longer answers the retry with
// another 429, which fails that one request instead of every page for the
// whole pause.
const (
	defaultRetryAfter = time.Second
	maxRetryAfter     = 10 * time.Second
)

// limiter is a token bucket shared by every TMDB API call the process makes:
// rate tokens a second, up to burst held, one spent per request. A 429 from
// TMDB pauses it for the answer's Retry-After (pause), holding every call
// until then.
type limiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	tokens  float64
	last    time.Time // when tokens was last refilled
	blocked time.Time // no request goes out before this
}

func newLimiter(rate, burst float64) *limiter {
	return &limiter{rate: rate, burst: burst, tokens: burst}
}

// wait blocks until the limiter lets one request out, or ctx ends.
func (l *limiter) wait(ctx context.Context) error {
	return waitFor(ctx, l.reserve)
}

// waitFor asks reserve for a token until it spends one, sleeping as long as
// it says between asks, or until ctx ends.
func waitFor(ctx context.Context, reserve func(time.Time) time.Duration) error {
	for {
		delay := reserve(time.Now())
		if delay <= 0 {
			return nil
		}
		if err := sleep(ctx, delay); err != nil {
			return err
		}
	}
}

// reserve spends a token at now and returns zero, or returns how long to
// wait before asking again: until a pause ends, or until a token refills.
func (l *limiter) reserve(now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Before(l.blocked) {
		return l.blocked.Sub(now)
	}
	if now.After(l.last) {
		l.tokens = min(l.burst, l.tokens+now.Sub(l.last).Seconds()*l.rate)
		l.last = now
	}
	if l.tokens >= 1 {
		l.tokens--
		return 0
	}
	return time.Duration((1 - l.tokens) / l.rate * float64(time.Second))
}

// pause holds every request for d from now, unless an earlier pause already
// holds them longer.
func (l *limiter) pause(now time.Time, d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if until := now.Add(d); until.After(l.blocked) {
		l.blocked = until
	}
}

// retryAfter reads a 429's Retry-After header at now, in seconds or as an
// HTTP date, as a pause between zero and maxRetryAfter; defaultRetryAfter
// when it is missing or unreadable.
func retryAfter(header string, now time.Time) time.Duration {
	d := defaultRetryAfter
	if seconds, err := strconv.Atoi(header); err == nil {
		// Clamped before scaling, so a huge value can't overflow Duration.
		d = time.Duration(min(max(seconds, 0), int(maxRetryAfter/time.Second))) * time.Second
	} else if at, err := http.ParseTime(header); err == nil {
		d = at.Sub(now)
	}
	return min(max(d, 0), maxRetryAfter)
}

// sleep waits for d, or until ctx ends.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// full reports whether l would let burst requests out at now: no pause holds
// it and its bucket has refilled.
func (l *limiter) full(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return !now.Before(l.blocked) && l.tokens+now.Sub(l.last).Seconds()*l.rate >= l.burst
}
