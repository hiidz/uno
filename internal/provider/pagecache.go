package provider

import (
	"container/list"
	"context"
	"fmt"
	"log"
	"slices"
	"sync"
	"time"
)

// catalogPageTTL is how long a finished catalog page is served from memory,
// so a client asking again gets a ranking at most this old.
const catalogPageTTL = 30 * time.Minute

// maxCatalogPageEntries bounds the page cache. A page is about twenty metas
// of a kilobyte or so each, overviews included, which holds the cache near
// 20 MB.
const maxCatalogPageEntries = 1_000

// catalogPageFetchTimeout bounds one shared page fetch, which runs detached
// from the request that started it (see pageCache.load).
const catalogPageFetchTimeout = 30 * time.Second

// pageCache holds finished catalog pages, each under the TMDB request that
// made it, for ttl, evicting the least recently used past max entries. N
// profiles showing one recipe cost TMDB one fetch per ttl rather than N.
//
// Callers asking for a key while its fetch is in flight wait for that fetch
// rather than starting their own. The fetch runs on a context detached from
// the caller that started it, so one client hanging up doesn't fail the
// others; a caller whose own context ends stops waiting. A failed fetch is
// not cached, nor a page its fetcher says not to keep.
type pageCache struct {
	mu       sync.Mutex
	ttl      time.Duration
	max      int
	order    *list.List // of *pageEntry, most recently used first
	entries  map[string]*list.Element
	inFlight map[string]*pageFetch
}

// pageFetcher fetches one page, reporting whether the cache should keep it.
type pageFetcher func(context.Context) (page CatalogPage, keep bool, err error)

// pageEntry is one cached page and the instant it stops being served.
type pageEntry struct {
	key     string
	page    CatalogPage
	expires time.Time
}

// pageFetch is one fetch in flight. page and err are set before done closes,
// and never after.
type pageFetch struct {
	done chan struct{}
	page CatalogPage
	err  error
}

func newPageCache(ttl time.Duration, maxEntries int) *pageCache {
	return &pageCache{
		ttl:      ttl,
		max:      maxEntries,
		order:    list.New(),
		entries:  make(map[string]*list.Element),
		inFlight: make(map[string]*pageFetch),
	}
}

// load returns key's page: cached while fresh, else from the fetch in flight
// for key, else from a new one, which caches what it gets. Every caller gets
// its own copy.
//
// A fetch runs with the key of the caller that started it, so a caller that
// waited on another's fetch and got that caller's key problem (IsKeyError)
// tries again, until it gets another answer or the fetch it waits on is its
// own: another caller with a key problem may have started the next one.
func (c *pageCache) load(ctx context.Context, key string, fetch pageFetcher) (CatalogPage, error) {
	for {
		page, waited, err := c.loadOnce(ctx, key, fetch)
		if !waited || !IsKeyError(err) {
			return page, err
		}
	}
}

// loadOnce is one try at load, reporting whether it waited on a fetch
// another caller started.
func (c *pageCache) loadOnce(ctx context.Context, key string, fetch pageFetcher) (CatalogPage, bool, error) {
	page, f, started := c.lookupOrStart(ctx, key, fetch)
	if f == nil {
		return page, false, nil
	}
	select {
	case <-f.done:
		return clonePage(f.page), !started, f.err
	case <-ctx.Done():
		return CatalogPage{}, !started, ctx.Err()
	}
}

// lookupOrStart returns a copy of key's fresh page, or else the fetch to
// wait for, starting one when none is in flight, and whether it started it.
func (c *pageCache) lookupOrStart(ctx context.Context, key string, fetch pageFetcher) (CatalogPage, *pageFetch, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if page, ok := c.fresh(key, time.Now()); ok {
		return clonePage(page), nil, false
	}
	f, ok := c.inFlight[key]
	if !ok {
		f = &pageFetch{done: make(chan struct{})}
		c.inFlight[key] = f
		go c.run(ctx, key, f, fetch)
	}
	return CatalogPage{}, f, !ok
}

// run is f's fetch, on a context that keeps ctx's values but not its
// cancellation, bounded by catalogPageFetchTimeout. It runs on its own
// goroutine, outside any handler's recover, so a panic in the fetch becomes
// f's error rather than ending the process; either way finish settles f.
func (c *pageCache) run(ctx context.Context, key string, f *pageFetch, fetch pageFetcher) {
	keep := false
	defer c.finish(key, f, &keep)
	defer recoverFetch(f)

	fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), catalogPageFetchTimeout)
	defer cancel()
	f.page, keep, f.err = fetch(fetchCtx)
}

// recoverFetch turns a panic in f's fetch into f's error. It must be
// deferred directly, for recover to see the panic.
func recoverFetch(f *pageFetch) {
	if rec := recover(); rec != nil {
		log.Printf("provider: catalog page fetch panicked: %v", rec)
		f.page, f.err = CatalogPage{}, fmt.Errorf("provider: catalog page fetch panicked: %v", rec)
	}
}

// finish ends f: it caches a successful page the fetch said to keep, then
// releases every caller waiting on f.
func (c *pageCache) finish(key string, f *pageFetch, keep *bool) {
	c.mu.Lock()
	delete(c.inFlight, key)
	if f.err == nil && *keep {
		c.store(key, f.page, time.Now())
	}
	c.mu.Unlock()
	close(f.done)
}

// fresh returns key's page when it is cached and unexpired at now, marking
// it most recently used, and drops it when it has expired. The caller holds
// c.mu.
func (c *pageCache) fresh(key string, now time.Time) (CatalogPage, bool) {
	el, ok := c.entries[key]
	if !ok {
		return CatalogPage{}, false
	}
	entry := el.Value.(*pageEntry)
	if now.After(entry.expires) {
		c.order.Remove(el)
		delete(c.entries, key)
		return CatalogPage{}, false
	}
	c.order.MoveToFront(el)
	return entry.page, true
}

// store caches page under key from now, as the most recently used entry,
// then evicts the least recently used past c.max. The caller holds c.mu.
func (c *pageCache) store(key string, page CatalogPage, now time.Time) {
	if el, ok := c.entries[key]; ok {
		c.order.Remove(el)
	}
	c.entries[key] = c.order.PushFront(&pageEntry{key: key, page: page, expires: now.Add(c.ttl)})
	for c.order.Len() > c.max {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.entries, oldest.Value.(*pageEntry).key)
	}
}

// clonePage copies a page down to each meta's genre list, so no caller can
// reach the cached copy.
func clonePage(page CatalogPage) CatalogPage {
	out := CatalogPage{Metas: slices.Clone(page.Metas), More: page.More}
	for i := range out.Metas {
		out.Metas[i].Genres = slices.Clone(out.Metas[i].Genres)
	}
	return out
}
