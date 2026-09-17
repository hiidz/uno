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
// Params) against a content Type, owned by a profile or shared publicly.
type Catalog struct {
	ID       uuid.UUID `json:"id"`
	Type     string    `json:"type"`
	Name     string    `json:"name"`
	Provider string    `json:"provider"`
	Params   string    `json:"params"`
	OwnerID  uuid.UUID `json:"owner_id"`
	IsPublic bool      `json:"is_public"`
	// CollectionID scopes this catalog to one collection (hidden from the
	// library, usable only in that collection's folders); nil means listed.
	CollectionID *uuid.UUID `json:"collection_id"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// HomeSortOrder is this catalog's position in its owner's home-screen
	// selection; nil means it isn't on the TV. Never on the wire — the
	// selection endpoints (GetCurrentCatalogSelection) return catalogs
	// already ordered by it.
	HomeSortOrder *int `json:"-"`
	// ShowInHome is only meaningful while HomeSortOrder is non-nil; it drives
	// the manifest's per-catalog genre extra (see buildManifest). Never on
	// the wire directly — SelectedCatalog carries its own copy for that.
	ShowInHome bool `json:"-"`
	// TakenFrom is the source catalog a Take copied this row from, kept only
	// to answer "you already took this" — never rendered as attribution.
	TakenFrom *uuid.UUID `json:"-"`
	// Fingerprint collapses duplicate community catalogs by recipe; never on
	// the wire.
	Fingerprint string `json:"-"`
	// IsDefault is never set true by any code path today — kept internal,
	// not exposed on the wire, so the API doesn't assert a value it isn't
	// actually tracking. See UpdateUserCatalog/UpdateUserCollection.
	IsDefault bool `json:"-"`
}

// Collection is a Nuvio home-screen collection: a titled group of Folders,
// owned by a profile or shared publicly.
type Collection struct {
	ID               uuid.UUID `json:"id"`
	Title            string    `json:"title"`
	OwnerID          uuid.UUID `json:"owner_id"`
	IsPublic         bool      `json:"is_public"`
	PinToTop         bool      `json:"pin_to_top"`
	ViewMode         string    `json:"view_mode"`
	ShowAllTab       bool      `json:"show_all_tab"`
	BackdropImageURL string    `json:"backdrop_image_url"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// Version increments on every content write (UpdateUserCollection) and
	// starts at 1 on insert — never touched by push. PushedVersion is the
	// Version push read and sent to Nuvio for this collection; nil means
	// never pushed. The frontend flags a pending change when the two differ.
	Version       int  `json:"version"`
	PushedVersion *int `json:"pushed_version"`
	// HomeSortOrder is this collection's position in its owner's home-screen
	// selection; nil means it isn't on the TV. Never on the wire — the
	// selection endpoint (GetCurrentCollectionSelection) returns collections
	// already ordered by it.
	HomeSortOrder *int `json:"-"`
	// TakenFrom is the source collection a Take copied this row from, kept
	// only to answer "you already took this" — never rendered as attribution.
	TakenFrom *uuid.UUID `json:"-"`
	// IsDefault is never set true by any code path today — kept internal,
	// not exposed on the wire, so the API doesn't assert a value it isn't
	// actually tracking. See UpdateUserCatalog/UpdateUserCollection.
	IsDefault bool `json:"-"`
	// focus_glow_enabled exists on this table (cosmetic Nuvio field, seen in
	// real collections_json pulls) and isn't modeled yet — stored/pushed
	// as-is, no editor. Same treatment as Folder's cosmetic fields below.
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
	// Additional cosmetic Nuvio fields exist on this table
	// (focus_gif_url, hero_video_url, title_logo_url, etc.)
	// and aren't modeled yet — stored/pushed as-is, no editor.
}

// FolderCatalog joins a Folder to one of its member Catalogs, in order.
type FolderCatalog struct {
	FolderID  uuid.UUID `json:"folder_id"`
	CatalogID uuid.UUID `json:"catalog_id"`
	SortOrder int       `json:"sort_order"`
}

// HTTP Inbound Model-------------------------

// CatalogForm is the create/update request body for a Catalog.
type CatalogForm struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Params   string `json:"params"`
	IsPublic bool   `json:"is_public"`
	// CollectionID scopes the catalog to one collection; nil (or absent on
	// the wire) means listed. On update, setting it demotes the catalog and
	// clearing it promotes — see CreateUserCatalog/UpdateUserCatalog.
	CollectionID *uuid.UUID `json:"collection_id"`
	// Fingerprint is computed server-side after validation, never accepted
	// from the client.
	Fingerprint string `json:"-"`
}

// CollectionForm is the create/update request body for a Collection,
// including its full set of folders.
type CollectionForm struct {
	Title            string       `json:"title"`
	IsPublic         bool         `json:"is_public"`
	PinToTop         bool         `json:"pin_to_top"`
	ViewMode         string       `json:"view_mode"`
	ShowAllTab       bool         `json:"show_all_tab"`
	BackdropImageURL string       `json:"backdrop_image_url"`
	Folders          []FolderData `json:"folders"`
}

// FolderData is one folder within a CollectionForm.
type FolderData struct {
	ID            *uuid.UUID  `json:"id,omitempty"` // nil = new folder, present = existing
	Title         string      `json:"title"`
	TileShape     string      `json:"tile_shape"`
	HideTitle     bool        `json:"hide_title"`
	CoverEmoji    string      `json:"cover_emoji"`
	CoverImageURL string      `json:"cover_image_url"`
	CatalogIDs    []uuid.UUID `json:"catalog_ids"` // ordered — index gives folder_catalogs.sort_order
}

// SelectedCatalogInput is one entry in a CatalogSelectionForm.
type SelectedCatalogInput struct {
	CatalogID  uuid.UUID `json:"catalog_id"`
	ShowInHome bool      `json:"show_in_home"`
}

// CatalogSelectionForm is the request body for setting a profile's active
// catalog selection.
type CatalogSelectionForm struct {
	Catalogs []SelectedCatalogInput `json:"catalogs"` // ordered — index gives catalogs.home_sort_order
}

// CollectionSelectionForm is the request body for setting a profile's
// active collection selection.
type CollectionSelectionForm struct {
	CollectionIDs []uuid.UUID `json:"collection_ids"` // ordered — index gives collections.home_sort_order
}

// HTTP Outbound Model-------------------------

// FolderWithCatalogs is a Folder plus the ordered IDs of its member
// catalogs.
type FolderWithCatalogs struct {
	Folder
	CatalogIDs []uuid.UUID `json:"catalog_ids"`
}

// CollectionWithFolders is a Collection plus its folders, each with their
// member catalog IDs, and every catalog those folders reference (listed or
// scoped) so the editor never needs the library to render a folder.
type CollectionWithFolders struct {
	Collection
	Folders  []FolderWithCatalogs `json:"folders"`
	Catalogs []Catalog            `json:"catalogs"`
}

// SelectedCatalog is a Catalog as it appears in a profile's active
// selection, carrying that selection's show-in-home flag.
type SelectedCatalog struct {
	Catalog
	ShowInHome bool `json:"show_in_home"`
}

// CommunityCatalog is a Catalog as it appears in the community list: public,
// owned by someone else, collapsed to one row per fingerprint, plus whether
// the caller has already taken a copy.
type CommunityCatalog struct {
	Catalog
	Taken bool `json:"taken"`
}

// CommunityCollection is a CollectionWithFolders as it appears in the
// community list: public, owned by someone else, plus whether the caller has
// already taken a copy. Unlike CommunityCatalog there is no fingerprint
// collapse — that's a catalog-only concept.
type CommunityCollection struct {
	CollectionWithFolders
	Taken bool `json:"taken"`
}
