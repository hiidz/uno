package vault

import (
	"time"

	"github.com/google/uuid"
)

// DB Table Models-------------------------

// Profile is a Uno profile row: one per Nuvio (user, profile index) pair,
// identified externally by Token. Never on the wire: the builder is told only
// its manifest URL.
type Profile struct {
	ID                uuid.UUID
	Token             string
	NuvioUserID       string
	NuvioProfileIndex int
	NuvioProfileUUID  string
}

// Catalog is a stored addon catalog: a named request recipe (Provider,
// Params) against a content Type, owned by a profile.
type Catalog struct {
	ID       uuid.UUID `json:"id"`
	Type     string    `json:"type"`
	Name     string    `json:"name"`
	Provider string    `json:"provider"`
	Params   string    `json:"params"`
	// OwnerID is never on the wire: the builder reads only its own rows.
	OwnerID uuid.UUID `json:"-"`
	// CollectionID scopes this catalog to one collection (hidden from the
	// library, usable only in that collection's folders); nil means listed.
	CollectionID *uuid.UUID `json:"collection_id"`
	// CreatedAt and UpdatedAt are never on the wire: the builder reads
	// neither.
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
	// HomeSortOrder is this catalog's position on its owner's home screen,
	// one numbering shared with the collections there; nil means it isn't on
	// Home. On the wire as home_position, null when off Home, which the builder
	// places its Home rows by.
	HomeSortOrder *int `json:"home_position"`
	// ShowInHome is whether this catalog on Home gets a home row, false for
	// Discover only. A read sets it only while HomeSortOrder is non-nil, so it
	// is false for every catalog off Home.
	ShowInHome bool `json:"show_in_home"`
	// Revision rises by one with each content write to the row, from 1 when it
	// is made. A catalog editor's save carries the revision it was built from,
	// and a save from any other is refused (UpdateUserCatalog).
	Revision int64 `json:"revision"`
	// RecipeHash is this catalog's recipe's hash (see RecipeHash), which
	// every catalog with the same recipe shares, computed when the row is
	// read; never on the wire.
	RecipeHash string `json:"-"`
	// SubKey is, for a catalog inside a subscribed collection, the key of the
	// snapshot catalog it was written from, which Update pairs it by; empty
	// otherwise. Never on the wire.
	SubKey string `json:"-"`
	// Publication is this catalog's own publication, and Subscription the
	// publication this listed catalog is a subscribed copy of; each is nil
	// when there is none.
	Publication  *PublicationState  `json:"publication"`
	Subscription *SubscriptionState `json:"subscription"`
}

// PublicationState is what a publisher's row shows of its live publication:
// its id, and whether the row has changed since it was last published, which
// is a hint to the publisher only.
type PublicationState struct {
	ID                  uuid.UUID `json:"id"`
	ChangedSincePublish bool      `json:"changed_since_publish"`
	// contentHash is the publication's content hash, which the row's own
	// snapshot is compared with.
	contentHash string
}

// SubscriptionState is what a subscribed copy shows of the publication it
// was subscribed from: its id, and whether a newer snapshot is published.
type SubscriptionState struct {
	PublicationID   uuid.UUID `json:"publication_id"`
	UpdateAvailable bool      `json:"update_available"`
}

// Collection is a Nuvio home-screen collection: a titled group of Folders,
// owned by a profile.
type Collection struct {
	ID               uuid.UUID `json:"id"`
	Title            string    `json:"title"`
	OwnerID          uuid.UUID `json:"-"` // never on the wire, as on Catalog
	PinToTop         bool      `json:"pin_to_top"`
	ViewMode         string    `json:"view_mode"`
	ShowAllTab       bool      `json:"show_all_tab"`
	BackdropImageURL string    `json:"backdrop_image_url"`
	// FocusGlowEnabled turns on Nuvio's TV focus glow on this collection's
	// home-screen folder cards.
	FocusGlowEnabled bool      `json:"focus_glow_enabled"`
	CreatedAt        time.Time `json:"-"` // never on the wire, as on Catalog
	UpdatedAt        time.Time `json:"-"`
	// HomeSortOrder is this collection's position on its owner's home
	// screen, one numbering shared with the catalogs there; nil means it isn't
	// on Home. On the wire as home_position, null when off Home.
	HomeSortOrder *int `json:"home_position"`
	// Revision rises by one with each content write to the row, from 1 when it
	// is made. A collection editor's save carries the revision it was built
	// from, and a save from any other is refused (UpdateUserCollection).
	Revision int64 `json:"revision"`
	// Publication is this collection's own publication, and Subscription the
	// publication it is a subscribed copy of; each is nil when there is none.
	Publication  *PublicationState  `json:"publication"`
	Subscription *SubscriptionState `json:"subscription"`
}

// Folder is one tile row within a Collection. Its CollectionID and SortOrder
// are never on the wire: a folder travels inside its collection, in order.
type Folder struct {
	ID           uuid.UUID `json:"id"`
	CollectionID uuid.UUID `json:"-"`
	Title        string    `json:"title"`
	SortOrder    int       `json:"-"`
	FolderArt
	// SubKey is, in a subscribed collection, the key of the snapshot folder
	// this one was written from, which Update pairs it by; empty otherwise.
	// Never on the wire.
	SubKey string `json:"-"`
}

// FolderArt is how a folder looks: everything it holds but its title, refs and
// identity. Folder, FolderData and BundleFolder embed it, so a row, a form, a
// bundle and a snapshot carry the same fields in the same order.
type FolderArt struct {
	TileShape     string `json:"tile_shape"`
	HideTitle     bool   `json:"hide_title"`
	CoverEmoji    string `json:"cover_emoji"`
	CoverImageURL string `json:"cover_image_url"`
	// FocusGIFURL is an animated GIF Nuvio plays over the folder's tile while
	// it's focused, when FocusGIFEnabled is set.
	FocusGIFURL     string `json:"focus_gif_url"`
	FocusGIFEnabled bool   `json:"focus_gif_enabled"`
	// HeroBackdropURL, HeroVideoURL and TitleLogoURL are the folder's hero
	// media for Nuvio's Modern Home layout.
	HeroBackdropURL string `json:"hero_backdrop_url"`
	HeroVideoURL    string `json:"hero_video_url"`
	TitleLogoURL    string `json:"title_logo_url"`
}

// FolderCatalog joins a Folder to one of its member Catalogs, in order.
type FolderCatalog struct {
	FolderID  uuid.UUID `json:"folder_id"`
	CatalogID uuid.UUID `json:"catalog_id"`
	SortOrder int       `json:"sort_order"`
	Genre     string    `json:"genre"`
}

// HTTP Inbound Model-------------------------

// CatalogForm is the create/update request body for a Catalog. Params reach
// the vault in canonical form: the API checks a client's recipe and replaces
// its params with provider.CanonicalParams before writing it, and the vault
// stores them as they come. A catalog it creates is listed; nothing changes
// a catalog's scope.
type CatalogForm struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Params   string `json:"params"`
}

// CollectionForm is the create/update request body for a Collection,
// including its full set of folders.
type CollectionForm struct {
	Title            string       `json:"title"`
	ViewMode         string       `json:"view_mode"`
	ShowAllTab       bool         `json:"show_all_tab"`
	BackdropImageURL string       `json:"backdrop_image_url"`
	FocusGlowEnabled bool         `json:"focus_glow_enabled"`
	Folders          []FolderData `json:"folders"`
	// CatalogEdits rewrites catalogs already scoped to this collection, in
	// the same transaction as the rest of the save. It is the only way such a
	// catalog is written once it exists (UpdateUserCatalog refuses one), so
	// an edit made in the collection editor and then discarded never reaches
	// the database. Update only: a collection being created has no scoped
	// catalogs yet.
	CatalogEdits []ScopedCatalogEdit `json:"catalog_edits"`
}

// ScopedCatalogEdit is one entry in CollectionForm.CatalogEdits: the new
// name and recipe for catalog ID, which must already be scoped to the
// collection being saved. Type and Provider must match the stored row — they
// are carried so the recipe can be validated before the vault reads it.
type ScopedCatalogEdit struct {
	ID       uuid.UUID `json:"id"`
	Type     string    `json:"type"`
	Provider string    `json:"provider"`
	Name     string    `json:"name"`
	Params   string    `json:"params"`
}

// FolderData is one folder within a CollectionForm.
type FolderData struct {
	ID    *uuid.UUID `json:"id,omitempty"` // nil = new folder, present = existing
	Title string     `json:"title"`
	FolderArt
	Catalogs []FolderCatalogRef `json:"catalogs"` // ordered — index gives folder_catalogs.sort_order
	// SubKey is written to a new folder's sub_key. Only a subscribe or an
	// Update sets it; never accepted from the client.
	SubKey string `json:"-"`
}

// FolderCatalogRef is one ordered entry in a folder's catalog list: either a
// reference to an existing catalog (CatalogID set) or an inline spec for a
// new catalog, created scoped to the enclosing collection in the same
// transaction as the folder write that references it (New set). Exactly one
// of the two is set — see CollectionForm.Validate.
//
// This is what makes "copy into this collection" and "new inside this
// collection" atomic with the collection's own save: the builder stages
// either kind of entry client-side with no request of its own, and the
// catalog row (for a New entry) is only ever written here, inside the
// transaction of the collection write that carries it — so discarding the
// edit instead of saving leaves nothing behind. A subscribe or a duplicate
// of a publication, or a Duplicate of a whole collection, writes its scoped catalog copies as New
// entries too.
//
// Genre narrows this one reference to a genre, by name: pushed as the folder
// source's "genre", which Nuvio sends back as the catalog's genre extra.
// Empty means unfiltered. It belongs to the reference, not the catalog, so
// the same catalog can be filtered differently in two folders, or appear
// twice in one folder under two genres.
type FolderCatalogRef struct {
	CatalogID *uuid.UUID        `json:"catalog_id,omitempty"`
	New       *NewScopedCatalog `json:"new,omitempty"`
	Genre     string            `json:"genre,omitempty"`
}

// CatalogRefs builds an ordered list of existing-catalog refs from ids —
// convenience for a caller assembling a FolderData programmatically without
// spelling out {CatalogID: &id} for each one.
func CatalogRefs(ids ...uuid.UUID) []FolderCatalogRef {
	refs := make([]FolderCatalogRef, len(ids))
	for i, id := range ids {
		refs[i] = FolderCatalogRef{CatalogID: &id}
	}
	return refs
}

// NewScopedCatalog is an inline catalog spec for FolderCatalogRef.New. Not
// home-eligible by construction — a scoped catalog never is (the CHECK on
// catalogs) — so no placement fields are accepted here at all.
type NewScopedCatalog struct {
	// Key is the client's handle for one staged catalog, unique within a
	// single save: every New entry carrying the same Key, in any folder and
	// under any genre, resolves to the one catalog the save creates for it.
	// It is never stored.
	Key      string `json:"key"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Params   string `json:"params"`
	// SubKey is written to the new row's sub_key. Only a subscribe or an
	// Update sets it; never accepted from the client.
	SubKey string `json:"-"`
}

// SelectedCatalogInput is one catalog on a profile's Home: whether it gets a
// home row or is in Discover only, and its Position, its place on Home in
// one numbering shared with the collections (catalogs.home_sort_order).
type SelectedCatalogInput struct {
	CatalogID  uuid.UUID `json:"catalog_id"`
	ShowInHome bool      `json:"show_in_home"`
	Position   int       `json:"position"`
}

// SelectedCollectionInput is one collection on a profile's Home: whether
// Nuvio shows it first, and its Position, its place on Home in one numbering
// shared with the catalogs (collections.home_sort_order). Push is the only
// writer of a collection's pin_to_top, and this is where it comes from.
type SelectedCollectionInput struct {
	CollectionID uuid.UUID `json:"collection_id"`
	PinToTop     bool      `json:"pin_to_top"`
	Position     int       `json:"position"`
}

// HTTP Outbound Model-------------------------

// FolderRef is one resolved entry in a folder's ordered catalog list: the
// catalog it references and the genre that reference is narrowed to ("" for
// unfiltered). The same catalog can appear more than once in one folder, each
// time with a different genre.
type FolderRef struct {
	CatalogID uuid.UUID `json:"catalog_id"`
	Genre     string    `json:"genre"`
}

// FolderWithCatalogs is a Folder plus its ordered catalog references.
type FolderWithCatalogs struct {
	Folder
	Refs []FolderRef `json:"refs"`
}

// CatalogIDs returns the catalog id of every ref in f, in order — repeats
// included when one catalog is referenced under more than one genre.
func (f FolderWithCatalogs) CatalogIDs() []uuid.UUID {
	ids := make([]uuid.UUID, len(f.Refs))
	for i, ref := range f.Refs {
		ids[i] = ref.CatalogID
	}
	return ids
}

// CollectionWithFolders is a Collection plus its folders, each with their
// member catalog IDs, and every catalog those folders reference (listed or
// scoped) so the editor never needs the library to render a folder.
type CollectionWithFolders struct {
	Collection
	Folders  []FolderWithCatalogs `json:"folders"`
	Catalogs []Catalog            `json:"catalogs"`
}
