package nuvio

import "time"

// Claims is the subset of a verified Nuvio JWT's claims Uno needs.
type Claims struct {
	Sub string
	// Email is the account's email address as the token carries it, "" when
	// it carries none. The access allowlist matches it (internal/api).
	Email string
	Exp   time.Time
}

// NuvioProfile is the subset of sync_pull_profiles's response fields Uno
// currently needs. See Nuvio's public API doc for the full response shape.
//
// UsesPrimaryAddons is set on a profile Nuvio's apps give profile 1's addon
// list in place of its own: an addon pushed to that profile is in a list no
// app reads. The avatar fields and PinEnabled are how Nuvio's apps draw the
// profile: a picture (AvatarURL, its own upload, or AvatarID, one of
// Nuvio's built-in avatars), else a circle in AvatarColorHex, and a lock
// when it has a PIN. A null avatar field reads as "".
type NuvioProfile struct {
	ID                string `json:"id"`
	UserID            string `json:"user_id"`
	ProfileIndex      int    `json:"profile_index"`
	Name              string `json:"name"`
	UsesPrimaryAddons bool   `json:"uses_primary_addons"`
	AvatarColorHex    string `json:"avatar_color_hex"`
	AvatarID          string `json:"avatar_id"`
	AvatarURL         string `json:"avatar_url"`
	PinEnabled        bool   `json:"pin_enabled"`
}

// NuvioAddon is one row from GET /rest/v1/addons — the subset Uno needs to
// merge its own manifest URL into a profile's existing addon list without
// disturbing the rest. The same shape is one entry of sync_push_addons'
// p_addons array, so the read and the write use this one type: the addon
// list Uno pushes is the list it pulled, with its own entry upserted.
type NuvioAddon struct {
	URL       string `json:"url"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	SortOrder int    `json:"sort_order"`
}
