// Package nuvio is the client for Nuvio's account API: verifying bearer
// tokens, and reading/writing a user's profiles, addons, and collections.
package nuvio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ErrNuvioRequestFailed wraps any non-2xx response or transport error from
// a call to Nuvio, so callers can distinguish "Nuvio itself failed" from
// other error causes with errors.Is.
var ErrNuvioRequestFailed = errors.New("nuvio: request failed")

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
	url := c.baseURL + "/rest/v1/rpc/sync_pull_profiles"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("apikey", c.publishableKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}
	defer resp.Body.Close()

	// sync_pull_profiles always returns data (a pull, not a push), so 200
	// is the only success case — unlike a push RPC, which can legitimately
	// return 204 No Content. Don't copy this check as-is for a push method.
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %s", ErrNuvioRequestFailed, resp.Status)
	}

	var profiles []NuvioProfile
	if err := json.NewDecoder(resp.Body).Decode(&profiles); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}
	// Defense in depth: sync_pull_profiles is documented as always returning
	// an array, but a nil slice here would marshal as `null` on the wire, and
	// the client's ProfilePicker treats `null` as "not loaded yet" rather than
	// "loaded, empty".
	if profiles == nil {
		profiles = []NuvioProfile{}
	}
	return profiles, nil
}

// doRPC POSTs body (marshaled to JSON) to {baseURL}/rest/v1/rpc/{rpc} with
// the caller's bearer token and the publishable key, and returns the raw
// response for the caller to inspect (status + body) — shared by every push/
// pull RPC below so the header/marshal boilerplate exists in one place.
func (c *Client) doRPC(ctx context.Context, accessToken, rpc string, body any) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}

	url := c.baseURL + "/rest/v1/rpc/" + rpc
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("apikey", c.publishableKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}
	return resp, nil
}

// ListAddons reads a profile's current addons via a direct table query (not
// an RPC) — the read half of the addons read-modify-write push cycle.
func (c *Client) ListAddons(ctx context.Context, accessToken string, profileID int) ([]NuvioAddon, error) {
	url := fmt.Sprintf("%s/rest/v1/addons?select=url,name,enabled,sort_order&profile_id=eq.%d&order=sort_order", c.baseURL, profileID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("apikey", c.publishableKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %s", ErrNuvioRequestFailed, resp.Status)
	}

	var addons []NuvioAddon
	if err := json.NewDecoder(resp.Body).Decode(&addons); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}
	return addons, nil
}

// PushAddons full-replaces the profile's addon list. Any addon omitted from
// addons is deleted server-side, so callers must pass the complete merged
// list, not just Uno's own entry. Success is 204, not 200 — unlike
// ListProfiles, which pulls data and expects 200.
func (c *Client) PushAddons(ctx context.Context, accessToken string, profileID int, addons []PushAddonInput) error {
	if addons == nil {
		addons = []PushAddonInput{}
	}
	body := struct {
		ProfileID int              `json:"p_profile_id"`
		Addons    []PushAddonInput `json:"p_addons"`
	}{profileID, addons}

	resp, err := c.doRPC(ctx, accessToken, "sync_push_addons", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %s", ErrNuvioRequestFailed, resp.Status)
	}

	var envelope []struct {
		CollectionsJSON []json.RawMessage `json:"collections_json"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNuvioRequestFailed, err)
	}
	if len(envelope) == 0 {
		return nil, nil
	}
	return envelope[0].CollectionsJSON, nil
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("%w: status %s", ErrNuvioRequestFailed, resp.Status)
	}
	return nil
}
