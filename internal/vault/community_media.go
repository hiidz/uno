package vault

import (
	"context"
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"
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
	db.mediaSites = sites
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

// withheld is what shared leaves out of s: empty when it leaves nothing out.
func (db *DB) withheld(s Snapshot) []WithheldAddress {
	if db.sharesMedia == nil {
		return []WithheldAddress{}
	}
	return s.withheld(db.sharesMedia)
}

// onSite reports whether raw is an https address on one of sites: a site
// name its host is, or "*." and a name its host is under (siteOf). Anything
// that doesn't parse as an https address with a host is on none.
func onSite(raw string, sites []string) bool {
	host, ok := siteOf(raw)
	if !ok {
		return false
	}
	return slices.ContainsFunc(sites, func(site string) bool {
		if under, ok := strings.CutPrefix(site, "*."); ok {
			return strings.HasSuffix(host, "."+under)
		}
		return host == site
	})
}

// siteOf is the host raw, an https address, names: lower-cased, without a
// trailing dot or port. It is false for anything else.
func siteOf(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return "", false
	}
	return strings.TrimSuffix(strings.ToLower(u.Hostname()), "."), true
}

// WithheldMedia is what Community would leave out of a collection: each
// media address it doesn't share, and the sites it shares pictures from,
// empty on a server that shares every site or none.
type WithheldMedia struct {
	Pictures []WithheldPicture `json:"pictures"`
	Sites    []string          `json:"sites"`
}

// WithheldPicture is one media address Community leaves out: the title of
// the folder it is in, or of the collection for its backdrop, the field it
// is in by its wire name, the address, and its site ("" for an address that
// names none).
type WithheldPicture struct {
	Title string `json:"title"`
	Field string `json:"field"`
	URL   string `json:"url"`
	Site  string `json:"site"`
}

// CollectionWithheldMedia is what Community would leave out of profileID's
// collection collectionID, as saved, by the same rule it shows and copies a
// publication by (shared): the collection's backdrop first, then each
// folder's in order, a picture used in several folders once per folder.
// It refuses a subscribed copy, which can't be published.
func (db *DB) CollectionWithheldMedia(ctx context.Context, profileID, collectionID uuid.UUID) (WithheldMedia, error) {
	tree, err := publishableCollection(ctx, db.conn, profileID, collectionID)
	if err != nil {
		return WithheldMedia{}, err
	}
	out := WithheldMedia{Pictures: []WithheldPicture{}, Sites: slices.Clone(db.mediaSites)}
	if out.Sites == nil {
		out.Sites = []string{}
	}
	if db.sharesMedia == nil {
		return out, nil
	}
	add := func(title, field, raw string) {
		if raw != "" && !db.sharesMedia(raw) {
			site, _ := siteOf(raw)
			out.Pictures = append(out.Pictures, WithheldPicture{Title: title, Field: field, URL: raw, Site: site})
		}
	}
	add(tree.Title, "backdrop_image_url", tree.BackdropImageURL)
	for _, f := range tree.Folders {
		add(f.Title, "cover_image_url", f.CoverImageURL)
		add(f.Title, "focus_gif_url", f.FocusGIFURL)
		add(f.Title, "hero_backdrop_url", f.HeroBackdropURL)
		add(f.Title, "title_logo_url", f.TitleLogoURL)
	}
	return out, nil
}
