package provider

import (
	"slices"
	"sync"
	"time"
)

// memo caches one TMDB lookup list per key. Every list it holds is small,
// changes on a timescale of months at the fastest, and is read on both the
// builder's picker routes and the recipe validation that runs on every
// save — so the same handful of responses would otherwise be re-fetched
// per request.
//
// clone is applied to every value on its way out. It is required, not an
// option: a memo of slices or maps without it hands every caller the same
// backing array, and one caller sorting or appending to what it got back
// would corrupt the cache for the rest.
type memo[V any] struct {
	mu      sync.RWMutex
	entries map[string]memoEntry[V]

	ttl   time.Duration // zero never expires
	clone func(V) V
}

// memoEntry is one cached value and the instant it stops being usable. A
// zero expires never does.
type memoEntry[V any] struct {
	value   V
	expires time.Time
}

// newMemo builds a memo whose entries live for ttl — zero for the process's
// lifetime — and pass through clone on every read.
func newMemo[V any](ttl time.Duration, clone func(V) V) *memo[V] {
	return &memo[V]{
		entries: make(map[string]memoEntry[V]),
		ttl:     ttl,
		clone:   clone,
	}
}

// load returns key's cached value, calling fetch to fill the entry on a
// miss or past an expiry.
//
// fetch runs with no lock held, so a slow TMDB call never blocks a reader.
// Two callers missing the same key at once therefore both fetch and the
// last to finish wins, which costs one redundant request and keeps the
// lock off the network. A failed fetch is not cached, so the next call
// retries.
func (m *memo[V]) load(key string, fetch func() (V, error)) (V, error) {
	if v, ok := m.lookup(key); ok {
		return v, nil
	}

	v, err := fetch()
	if err != nil {
		var zero V
		return zero, err
	}

	var expires time.Time
	if m.ttl > 0 {
		expires = time.Now().Add(m.ttl)
	}

	m.mu.Lock()
	m.entries[key] = memoEntry[V]{value: v, expires: expires}
	m.mu.Unlock()

	return m.clone(v), nil
}

// lookup returns a clone of key's value when the entry is present and
// unexpired.
func (m *memo[V]) lookup(key string) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.entries[key]
	if !ok || (!entry.expires.IsZero() && time.Now().After(entry.expires)) {
		var zero V
		return zero, false
	}
	return m.clone(entry.value), true
}

// cloneCertifications deep-copies TMDB's per-country certification map.
// maps.Clone would copy only the map, leaving every country's slice shared
// with the cache — this is the one memo value whose clone isn't a single
// slices.Clone.
func cloneCertifications(byCountry map[string][]Certification) map[string][]Certification {
	out := make(map[string][]Certification, len(byCountry))
	for country, scale := range byCountry {
		out[country] = slices.Clone(scale)
	}
	return out
}
