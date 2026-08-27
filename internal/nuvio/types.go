package nuvio

import "time"

// Claims is the subset of a verified Nuvio JWT's claims Uno needs.
type Claims struct {
	Sub string
	Exp time.Time
}

// NuvioProfile is the subset of sync_pull_profiles's response fields Uno
// currently needs. See Nuvio's public API doc for the full response shape.
type NuvioProfile struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	ProfileIndex int    `json:"profile_index"`
	Name         string `json:"name"`
}

// NuvioAddon is one row from GET /rest/v1/addons — the subset Uno needs to
// merge its own manifest URL into a profile's existing addon list without
// disturbing the rest.
type NuvioAddon struct {
	URL       string `json:"url"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	SortOrder int    `json:"sort_order"`
}

// PushAddonInput is the request-side shape for one entry of
// sync_push_addons' p_addons array.
type PushAddonInput struct {
	URL       string `json:"url"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	SortOrder int    `json:"sort_order"`
}

// CatalogSource is one folder catalogSources[] entry — a reference to one of
// this addon's catalogs, by addon+type+catalog id. Built against the field
// name the public doc documents; real pulled data uses "sources" for the
// containing array name but the entry shape itself is unaffected.
type CatalogSource struct {
	AddonID   string `json:"addonId"`
	Type      string `json:"type"`
	CatalogID string `json:"catalogId"`
}

// PushFolder is one collection folder in push/pull's camelCase wire shape —
// distinct from vault.Folder, which is snake_case and DB-column-shaped.
type PushFolder struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	CoverImageURL  string          `json:"coverImageUrl,omitempty"`
	CoverEmoji     string          `json:"coverEmoji,omitempty"`
	TileShape      string          `json:"tileShape"`
	HideTitle      bool            `json:"hideTitle"`
	CatalogSources []CatalogSource `json:"catalogSources"`
}

// PushCollection is one collection in push/pull's camelCase wire shape.
type PushCollection struct {
	ID               string       `json:"id"`
	Title            string       `json:"title"`
	BackdropImageURL string       `json:"backdropImageUrl,omitempty"`
	PinToTop         bool         `json:"pinToTop"`
	ViewMode         string       `json:"viewMode"`
	ShowAllTab       bool         `json:"showAllTab"`
	Folders          []PushFolder `json:"folders"`
}
