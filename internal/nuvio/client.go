// Package nuvio is the client for Nuvio's account API: verifying bearer
// tokens, and reading/writing a user's profiles, addons, collections and
// home order.
package nuvio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hiidz/uno/internal/jsonwire"
)

// ErrNuvioRequestFailed wraps any non-2xx response or transport error from
// a call to Nuvio, so callers can distinguish "Nuvio itself failed" from
// other error causes with errors.Is.
var ErrNuvioRequestFailed = errors.New("nuvio: request failed")

// maxResponseBytes caps how much of a Nuvio response this package will read
// before giving up. A profile list or a collections blob is orders of
// magnitude under it; the cap is there so a malfunctioning or hostile
// upstream can't make Uno buffer without limit. A truncated body fails the
// decode, which is already an ErrNuvioRequestFailed.
const maxResponseBytes = 8 << 20 // 8 MiB

// userAgent names Uno on every request it makes to Nuvio. Nuvio's audit log
// of addon and collection pushes records the caller's User-Agent, so Uno's
// pushes read there as Uno's.
const userAgent = "Uno/1.0.0"

// Client wraps a Verifier with the publishable key and HTTP client needed
// to call Nuvio's authenticated REST/RPC surface on a caller's behalf,
// forwarding whatever bearer token that caller already presented. It never
// mints or stores a token of its own.
type Client struct {
	*Verifier
	publishableKey string
	http           *http.Client
}

// NewClient builds a Client that verifies tokens against baseURL's JWKS and
// calls its REST/RPC surface using publishableKey.
func NewClient(baseURL, publishableKey string) *Client {
	return &Client{
		Verifier:       NewVerifier(baseURL),
		publishableKey: publishableKey,
		http:           &http.Client{Timeout: 10 * time.Second},
	}
}

// ListProfiles calls POST /rest/v1/rpc/sync_pull_profiles with accessToken
// forwarded as-is. No request body — this RPC takes none.
func (c *Client) ListProfiles(ctx context.Context, accessToken string) ([]NuvioProfile, error) {
	resp, err := c.doRPC(ctx, accessToken, "sync_pull_profiles", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// sync_pull_profiles always returns data (a pull, not a push), so 200
	// is the only success case — unlike a push RPC, which can legitimately
	// return 204 No Content. Don't copy this check as-is for a push method.
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %s", ErrNuvioRequestFailed, resp.Status)
	}

	var profiles []NuvioProfile
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&profiles); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}
	// Defense in depth: sync_pull_profiles is documented as always returning
	// an array, but a nil slice here would marshal as `null` on the wire, and
	// the client's ProfilePicker treats `null` as "not loaded yet" rather than
	// "loaded, empty".
	return jsonwire.OrEmpty(profiles), nil
}

// AvatarImages calls POST /rest/v1/rpc/get_avatar_catalog, Nuvio's list of
// built-in profile avatars, and returns each one's image URL by avatar id.
// The RPC needs no sign-in; accessToken is forwarded like any other call's.
func (c *Client) AvatarImages(ctx context.Context, accessToken string) (map[string]string, error) {
	resp, err := c.doRPC(ctx, accessToken, "get_avatar_catalog", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %s", ErrNuvioRequestFailed, resp.Status)
	}

	var entries []avatarEntry
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&entries); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}
	return avatarImageURLs(c.baseURL, entries), nil
}

// avatarEntry is the part of a get_avatar_catalog row an image URL needs.
type avatarEntry struct {
	ID          string `json:"id"`
	StoragePath string `json:"storage_path"`
}

// avatarImageURLs is each entry's image by its id: Nuvio's public storage
// bucket "avatars" at the entry's storage_path, as Nuvio's own apps build it
// (NuvioMobile's ProfileModels). An entry without a path has no image.
func avatarImageURLs(baseURL string, entries []avatarEntry) map[string]string {
	images := make(map[string]string, len(entries))
	for _, e := range entries {
		if path := strings.TrimLeft(e.StoragePath, "/"); path != "" {
			images[e.ID] = baseURL + "/storage/v1/object/public/avatars/" + path
		}
	}
	return images
}

// do issues one authenticated request against Nuvio: method and path as
// given, the caller's bearer token and the publishable key attached, and
// body marshaled to JSON when it is non-nil — a nil body sends none and no
// Content-Type, which is what the GET and the body-less RPC need. Returns
// the raw response for the caller to inspect (status + body); every call in
// this file goes through here so the header and marshal boilerplate exists
// in one place.
func (c *Client) do(ctx context.Context, method, accessToken, path string, body any) (*http.Response, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, payload)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("apikey", c.publishableKey)
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}
	return resp, nil
}

// doRPC POSTs body to {baseURL}/rest/v1/rpc/{rpc}. A nil body posts nothing,
// which is what sync_pull_profiles takes.
func (c *Client) doRPC(ctx context.Context, accessToken, rpc string, body any) (*http.Response, error) {
	return c.do(ctx, http.MethodPost, accessToken, "/rest/v1/rpc/"+rpc, body)
}

// ListAddons reads a profile's current addons via a direct table query (not
// an RPC) — the read half of the addons read-modify-write push cycle.
func (c *Client) ListAddons(ctx context.Context, accessToken string, profileID int) ([]NuvioAddon, error) {
	path := fmt.Sprintf("/rest/v1/addons?select=url,name,enabled,sort_order&profile_id=eq.%d&order=sort_order", profileID)
	resp, err := c.do(ctx, http.MethodGet, accessToken, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %s", ErrNuvioRequestFailed, resp.Status)
	}

	var addons []NuvioAddon
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&addons); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}
	return addons, nil
}

// PushAddons full-replaces the profile's addon list. Any addon omitted from
// addons is deleted server-side, so callers must pass the complete merged
// list, not just Uno's own entry. Success is 204, not 200 — unlike
// ListProfiles, which pulls data and expects 200.
func (c *Client) PushAddons(ctx context.Context, accessToken string, profileID int, addons []NuvioAddon) error {
	if addons == nil {
		addons = []NuvioAddon{}
	}
	body := struct {
		ProfileID int          `json:"p_profile_id"`
		Addons    []NuvioAddon `json:"p_addons"`
	}{profileID, addons}

	resp, err := c.doRPC(ctx, accessToken, "sync_push_addons", body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("%w: status %s", ErrNuvioRequestFailed, resp.Status)
	}
	return nil
}

// PullCollections reads a profile's current collections blob. The RPC
// returns an array wrapping the blob ([{profile_id, collections_json,
// updated_at}]), not the blob directly — a profile with no collections ever
// pushed returns an empty array, not a row with an empty collections_json,
// so both "no row" and "row with []" collapse to a nil/empty result here.
func (c *Client) PullCollections(ctx context.Context, accessToken string, profileID int) ([]json.RawMessage, error) {
	body := struct {
		ProfileID int `json:"p_profile_id"`
	}{profileID}

	resp, err := c.doRPC(ctx, accessToken, "sync_pull_collections", body)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %s", ErrNuvioRequestFailed, resp.Status)
	}

	var envelope []struct {
		CollectionsJSON []json.RawMessage `json:"collections_json"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}
	if len(envelope) == 0 {
		return nil, nil
	}
	return envelope[0].CollectionsJSON, nil
}

// homeOrderPlatform is the platform Nuvio TV, mobile and desktop all keep a
// profile's home-order list under. The vendor doc's "tv" platform is a
// leftover no Nuvio app reads.
const homeOrderPlatform = "home_catalog_shared"

// PullHomeOrder reads a profile's home-order list: the settings_json object
// Nuvio's apps order their home screen rows by, as Nuvio sent it. A profile
// whose list was never saved has no row, which comes back as nil, and so
// does a null one. A settings_json sent as a JSON string is the object that
// string holds.
func (c *Client) PullHomeOrder(ctx context.Context, accessToken string, profileID int) (json.RawMessage, error) {
	body := struct {
		ProfileID int    `json:"p_profile_id"`
		Platform  string `json:"p_platform"`
	}{profileID, homeOrderPlatform}

	resp, err := c.doRPC(ctx, accessToken, "sync_pull_home_catalog_settings", body)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %s", ErrNuvioRequestFailed, resp.Status)
	}

	var envelope []struct {
		SettingsJSON json.RawMessage `json:"settings_json"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}
	if len(envelope) == 0 {
		return nil, nil
	}
	return settingsObject(envelope[0].SettingsJSON), nil
}

// settingsObject is raw as an object's bytes: the object a JSON string holds,
// nil for null, raw itself otherwise.
func settingsObject(raw json.RawMessage) json.RawMessage {
	var encoded string
	if json.Unmarshal(raw, &encoded) == nil {
		return json.RawMessage(encoded)
	}
	return raw
}

// PushHomeOrder full-replaces a profile's home-order list with settings, a
// settings_json object; nil pushes {}. Any row or setting omitted from
// settings is gone from every Nuvio app once it next reads the list, so
// callers must pass the complete merged object. Success is 204.
func (c *Client) PushHomeOrder(ctx context.Context, accessToken string, profileID int, settings json.RawMessage) error {
	if len(settings) == 0 {
		settings = json.RawMessage(`{}`)
	}
	body := struct {
		ProfileID    int             `json:"p_profile_id"`
		Platform     string          `json:"p_platform"`
		SettingsJSON json.RawMessage `json:"p_settings_json"`
	}{profileID, homeOrderPlatform, settings}

	resp, err := c.doRPC(ctx, accessToken, "sync_push_home_catalog_settings", body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("%w: status %s", ErrNuvioRequestFailed, resp.Status)
	}
	return nil
}

// PushCollections full-replaces the profile's collections blob. Any
// collection omitted from collections is deleted server-side, so callers
// must pass the complete merged blob — Uno's own collections plus every
// collection it pulled and doesn't own, byte-for-byte. Success is 204.
func (c *Client) PushCollections(ctx context.Context, accessToken string, profileID int, collections []json.RawMessage) error {
	if collections == nil {
		collections = []json.RawMessage{}
	}
	body := struct {
		ProfileID       int               `json:"p_profile_id"`
		CollectionsJSON []json.RawMessage `json:"p_collections_json"`
	}{profileID, collections}

	resp, err := c.doRPC(ctx, accessToken, "sync_push_collections", body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("%w: status %s", ErrNuvioRequestFailed, resp.Status)
	}
	return nil
}
