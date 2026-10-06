package vault

import (
	"time"

	"github.com/google/uuid"
)

// DB Table Models-------------------------

// Profile is a Uno profile row: one per Nuvio (user, profile index) pair,
// identified externally by Token.
type Profile struct {
	ID                uuid.UUID `json:"id"`
	Token             string    `json:"token"`
	NuvioUserID       string    `json:"nuvio_user_id"`
	NuvioProfileIndex int       `json:"nuvio_profile_index"`
	NuvioProfileUUID  string    `json:"nuvio_profile_uuid"`
}

// Catalog is a stored addon catalog: a named request recipe (Provider,
// Params) against a content Type, owned by a profile. Type, Provider and
// Params are its recipe's, read from the recipes row RecipeHash names.
type Catalog struct {
	ID       uuid.UUID `json:"id"`
	Type     string    `json:"type"`
	Name     string    `json:"name"`
	Provider string    `json:"provider"`
	Params   string    `json:"params"`
	OwnerID  uuid.UUID `json:"owner_id"`
	// CollectionID scopes this catalog to one collection (hidden from the
	// library, usable only in that collection's folders); nil means listed.
	CollectionID *uuid.UUID `json:"collection_id"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// HomeSortOrder is this catalog's position on its owner's home screen,
	// one numbering shared with the collections there; nil means it isn't on
	// Home. On the wire as home_position, which the builder merges the two
	// selection reads by.
	HomeSortOrder *int `json:"home_position,omitempty"`
	// ShowInHome is only meaningful while HomeSortOrder is non-nil; it drives
	// the manifest's per-catalog genre extra (see buildManifest). Never on
	// the wire directly — SelectedCatalog carries its own copy for that.
	ShowInHome bool `json:"-"`
	// RecipeHash names this catalog's recipes row (see RecipeHash), which
	// every catalog with the same recipe shares; never on the wire.
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
	// PublisherUnpublished is set on a listed catalog that was a subscribed
	// copy until its publisher unpublished it, until it is next saved.
	PublisherUnpublished bool `json:"publisher_unpublished"`
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
	OwnerID          uuid.UUID `json:"owner_id"`
	PinToTop         bool      `json:"pin_to_top"`
	ViewMode         string    `json:"view_mode"`
	ShowAllTab       bool      `json:"show_all_tab"`
	BackdropImageURL string    `json:"backdrop_image_url"`
	// FocusGlowEnabled turns on Nuvio's TV focus glow on this collection's
	// home-screen folder cards.
	FocusGlowEnabled bool      `json:"focus_glow_enabled"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// HomeSortOrder is this collection's position on its owner's home
	// screen, one numbering shared with the catalogs there; nil means it isn't
	// on Home. On the wire as home_position.
	HomeSortOrder *int `json:"home_position,omitempty"`
	// Publication is this collection's own publication, and Subscription the
	// publication it is a subscribed copy of; each is nil when there is none.
	Publication  *PublicationState  `json:"publication"`
	Subscription *SubscriptionState `json:"subscription"`
	// PublisherUnpublished is set on a collection that was a subscribed copy
	// until its publisher unpublished it, until it is next saved.
	PublisherUnpublished bool `json:"publisher_unpublished"`
}

// Folder is one tile row within a Collection.
type Folder struct {
	ID            uuid.UUID `json:"id"`
	CollectionID  uuid.UUID `json:"collection_id"`
	Title         string    `json:"title"`
	SortOrder     int       `json:"sort_order"`
	TileShape     string    `json:"tile_shape"`
	HideTitle     bool      `json:"hide_title"`
	CoverEmoji    string    `json:"cover_emoji"`
	CoverImageURL string    `json:"cover_image_url"`
	// FocusGIFURL is an animated GIF Nuvio plays over the folder's tile while
	// it's focused, when FocusGIFEnabled is set.
	FocusGIFURL     string `json:"focus_gif_url"`
	FocusGIFEnabled bool   `json:"focus_gif_enabled"`
	// HeroBackdropURL, HeroVideoURL and TitleLogoURL are the folder's hero
	// media for Nuvio's Modern Home layout.
	HeroBackdropURL string `json:"hero_backdrop_url"`
	HeroVideoURL    string `json:"hero_video_url"`
	TitleLogoURL    string `json:"title_logo_url"`
	// SubKey is, in a subscribed collection, the key of the snapshot folder
	// this one was written from, which Update pairs it by; empty otherwise.
	// Never on the wire.
	SubKey string `json:"-"`
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
// stores them as they come.
type CatalogForm struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Params   string `json:"params"`
	// CollectionID scopes a new catalog to one collection; nil (or absent on
	// the wire) means listed. Only CreateUserCatalog reads it: an update
	// never changes a catalog's scope.
	CollectionID *uuid.UUID `json:"collection_id"`
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

// recipeHash is the hash of the recipe e writes.
func (e ScopedCatalogEdit) recipeHash() string {
	return RecipeHash(e.Type, e.Provider, e.Params)
}

// FolderData is one folder within a CollectionForm.
type FolderData struct {
	ID              *uuid.UUID         `json:"id,omitempty"` // nil = new folder, present = existing
	Title           string             `json:"title"`
	TileShape       string             `json:"tile_shape"`
	HideTitle       bool               `json:"hide_title"`
	CoverEmoji      string             `json:"cover_emoji"`
	CoverImageURL   string             `json:"cover_image_url"`
	FocusGIFURL     string             `json:"focus_gif_url"`
	FocusGIFEnabled bool               `json:"focus_gif_enabled"`
	HeroBackdropURL string             `json:"hero_backdrop_url"`
	HeroVideoURL    string             `json:"hero_video_url"`
	TitleLogoURL    string             `json:"title_logo_url"`
	Catalogs        []FolderCatalogRef `json:"catalogs"` // ordered — index gives folder_catalogs.sort_order
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
// entries too. See docs/frontend.md's "Three sources for a folder's catalog".
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

// CatalogSelectionForm is a profile's Home catalogs, in Position order.
type CatalogSelectionForm struct {
	Catalogs []SelectedCatalogInput `json:"catalogs"`
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

// CollectionSelectionForm is a profile's Home collections, in Position order.
type CollectionSelectionForm struct {
	Collections []SelectedCollectionInput `json:"collections"`
}

// CatalogIDs is the id of every catalog in f, in order.
func (f CatalogSelectionForm) CatalogIDs() []uuid.UUID {
	ids := make([]uuid.UUID, len(f.Catalogs))
	for i, c := range f.Catalogs {
		ids[i] = c.CatalogID
	}
	return ids
}

// CollectionIDs is the id of every collection in f, in order.
func (f CollectionSelectionForm) CollectionIDs() []uuid.UUID {
	ids := make([]uuid.UUID, len(f.Collections))
	for i, c := range f.Collections {
		ids[i] = c.CollectionID
	}
	return ids
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
	// NeedsPush is whether this collection is on Home and what push would
	// send for it now differs from what push last sent: Nuvio holds a stale
	// copy until the next push. Always false off Home.
	NeedsPush bool `json:"needs_push"`
}

// SelectedCatalog is a Catalog as it appears in a profile's active
// selection, carrying that selection's show-in-home flag.
type SelectedCatalog struct {
	Catalog
	ShowInHome bool `json:"show_in_home"`
}
