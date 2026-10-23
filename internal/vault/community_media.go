package vault

import (
	"net/url"
	"slices"
	"strings"
)

// WithholdCommunityMedia makes Community leave every media address out of
// what it shows and copies: the list, a publication's page, what an Update
// would change, and the copies Add, Duplicate and Update write. Publishers'
// own rows and the snapshots as stored keep theirs, so a server that shares
// them again shows them again. It is set once, before the DB serves anything.
func (db *DB) WithholdCommunityMedia() {
	db.sharesMedia = func(string) bool { return false }
}

// ShareCommunityMediaFrom is WithholdCommunityMedia for only the media
// addresses whose site isn't one of sites (onSite). sites are lower-cased
// site names, or "*." and a name. It is set once, before the DB serves
// anything.
func (db *DB) ShareCommunityMediaFrom(sites []string) {
	sites = slices.Clone(sites)
	db.sharesMedia = func(raw string) bool { return onSite(raw, sites) }
}

// shared is s as Community shows and copies it: s, or s without the media
// addresses the server doesn't share.
func (db *DB) shared(s Snapshot) Snapshot {
	if db.sharesMedia == nil {
		return s
	}
	return s.withMedia(db.sharesMedia)
}

// onSite reports whether raw is an https address on one of sites: a site
// name its host is, ignoring case and a trailing dot, or "*." and a name its
// host is under. The port doesn't matter. Anything that doesn't parse as an
// https address with a host is on none.
func onSite(raw string, sites []string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	return slices.ContainsFunc(sites, func(site string) bool {
		if under, ok := strings.CutPrefix(site, "*."); ok {
			return strings.HasSuffix(host, "."+under)
		}
		return host == site
	})
}
