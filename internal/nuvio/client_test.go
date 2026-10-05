package nuvio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
)

// nuvioRequest is what a fake Nuvio server saw of the last request it served.
type nuvioRequest struct {
	method, uri, contentType, authorization, apikey, userAgent, body string
}

// fakeNuvioServer answers every request with status and body, recording the
// request, and returns a Client pointed at it.
func fakeNuvioServer(t *testing.T, status int, body string) (*Client, *nuvioRequest) {
	t.Helper()
	var got nuvioRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}
		got = nuvioRequest{
			method:        r.Method,
			uri:           r.URL.RequestURI(),
			contentType:   r.Header.Get("Content-Type"),
			authorization: r.Header.Get("Authorization"),
			apikey:        r.Header.Get("apikey"),
			userAgent:     r.Header.Get("User-Agent"),
			body:          string(raw),
		}
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "publishable-key"), &got
}

// clientCall is one Client method, called against profile index 3 where it
// takes one, with its error as the only result the status tests look at.
type clientCall struct {
	name string
	call func(c *Client) error
}

var (
	listProfiles = clientCall{"ListProfiles", func(c *Client) error {
		_, err := c.ListProfiles(context.Background(), "access-token")
		return err
	}}
	listAddons = clientCall{"ListAddons", func(c *Client) error {
		_, err := c.ListAddons(context.Background(), "access-token", 3)
		return err
	}}
	pullCollections = clientCall{"PullCollections", func(c *Client) error {
		_, err := c.PullCollections(context.Background(), "access-token", 3)
		return err
	}}
	pushAddons = clientCall{"PushAddons", func(c *Client) error {
		return c.PushAddons(context.Background(), "access-token", 3, nil)
	}}
	avatarImages = clientCall{"AvatarImages", func(c *Client) error {
		_, err := c.AvatarImages(context.Background(), "access-token")
		return err
	}}
	pushCollections = clientCall{"PushCollections", func(c *Client) error {
		return c.PushCollections(context.Background(), "access-token", 3, nil)
	}}
	pullHomeOrder = clientCall{"PullHomeOrder", func(c *Client) error {
		_, err := c.PullHomeOrder(context.Background(), "access-token", 3)
		return err
	}}
	pushHomeOrder = clientCall{"PushHomeOrder", func(c *Client) error {
		return c.PushHomeOrder(context.Background(), "access-token", 3, nil)
	}}
)

// TestClientSendsNuvioRequests pins each method's request: the method and
// path Nuvio serves it on, the caller's token, the publishable key and Uno's
// User-Agent on every call, and the exact JSON body, with a Content-Type only
// when there is one. A nil list pushes as [], never null: the push is a full replace, and
// the empty array is what says "no addons".
func TestClientSendsNuvioRequests(t *testing.T) {
	bigNumber := json.RawMessage(`{"id":"foreign","n":12345678901234567890}`)
	tests := []struct {
		name     string
		status   int
		call     func(c *Client) error
		wantReq  nuvioRequest
		wantBody string
	}{
		{
			name: "ListProfiles", status: http.StatusOK,
			call:    listProfiles.call,
			wantReq: nuvioRequest{method: http.MethodPost, uri: "/rest/v1/rpc/sync_pull_profiles"},
		},
		{
			name: "ListAddons", status: http.StatusOK,
			call: listAddons.call,
			wantReq: nuvioRequest{
				method: http.MethodGet,
				uri:    "/rest/v1/addons?select=url,name,enabled,sort_order&profile_id=eq.3&order=sort_order",
			},
		},
		{
			name: "AvatarImages", status: http.StatusOK,
			call:    avatarImages.call,
			wantReq: nuvioRequest{method: http.MethodPost, uri: "/rest/v1/rpc/get_avatar_catalog"},
		},
		{
			name: "PullCollections", status: http.StatusOK,
			call:     pullCollections.call,
			wantReq:  nuvioRequest{method: http.MethodPost, uri: "/rest/v1/rpc/sync_pull_collections"},
			wantBody: `{"p_profile_id":3}`,
		},
		{
			name: "PushAddons", status: http.StatusNoContent,
			call: func(c *Client) error {
				return c.PushAddons(context.Background(), "access-token", 3, []NuvioAddon{
					{URL: "https://other.example/manifest.json", Name: "Other", Enabled: false, SortOrder: 0},
					{URL: "https://uno.example/u/tok/manifest.json", Name: "Uno Catalog", Enabled: true, SortOrder: 1},
				})
			},
			wantReq: nuvioRequest{method: http.MethodPost, uri: "/rest/v1/rpc/sync_push_addons"},
			wantBody: `{"p_profile_id":3,"p_addons":[` +
				`{"url":"https://other.example/manifest.json","name":"Other","enabled":false,"sort_order":0},` +
				`{"url":"https://uno.example/u/tok/manifest.json","name":"Uno Catalog","enabled":true,"sort_order":1}]}`,
		},
		{
			name: "PushAddons nil", status: http.StatusNoContent,
			call:     pushAddons.call,
			wantReq:  nuvioRequest{method: http.MethodPost, uri: "/rest/v1/rpc/sync_push_addons"},
			wantBody: `{"p_profile_id":3,"p_addons":[]}`,
		},
		{
			name: "PushCollections", status: http.StatusNoContent,
			call: func(c *Client) error {
				return c.PushCollections(context.Background(), "access-token", 3, []json.RawMessage{bigNumber})
			},
			wantReq:  nuvioRequest{method: http.MethodPost, uri: "/rest/v1/rpc/sync_push_collections"},
			wantBody: `{"p_profile_id":3,"p_collections_json":[{"id":"foreign","n":12345678901234567890}]}`,
		},
		{
			name: "PushCollections nil", status: http.StatusNoContent,
			call:     pushCollections.call,
			wantReq:  nuvioRequest{method: http.MethodPost, uri: "/rest/v1/rpc/sync_push_collections"},
			wantBody: `{"p_profile_id":3,"p_collections_json":[]}`,
		},
		{
			name: "PullHomeOrder", status: http.StatusOK,
			call:     pullHomeOrder.call,
			wantReq:  nuvioRequest{method: http.MethodPost, uri: "/rest/v1/rpc/sync_pull_home_catalog_settings"},
			wantBody: `{"p_profile_id":3,"p_platform":"home_catalog_shared"}`,
		},
		{
			name: "PushHomeOrder", status: http.StatusNoContent,
			call: func(c *Client) error {
				return c.PushHomeOrder(context.Background(), "access-token", 3, json.RawMessage(`{"items":[{"order":0,"n":12345678901234567890}]}`))
			},
			wantReq:  nuvioRequest{method: http.MethodPost, uri: "/rest/v1/rpc/sync_push_home_catalog_settings"},
			wantBody: `{"p_profile_id":3,"p_platform":"home_catalog_shared","p_settings_json":{"items":[{"order":0,"n":12345678901234567890}]}}`,
		},
		{
			name: "PushHomeOrder nil", status: http.StatusNoContent,
			call:     pushHomeOrder.call,
			wantReq:  nuvioRequest{method: http.MethodPost, uri: "/rest/v1/rpc/sync_push_home_catalog_settings"},
			wantBody: `{"p_profile_id":3,"p_platform":"home_catalog_shared","p_settings_json":{}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, got := fakeNuvioServer(t, tc.status, "[]")
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}

			want := tc.wantReq
			want.authorization = "Bearer access-token"
			want.apikey = "publishable-key"
			want.userAgent = userAgent
			want.body = tc.wantBody
			if tc.wantBody != "" {
				want.contentType = "application/json"
			}
			if *got != want {
				t.Fatalf("request = %+v\nwant      %+v", *got, want)
			}
		})
	}
}

// TestClientSuccessStatus pins the one status each method accepts: 200 for
// the pulls, which always carry data, and 204 for the pushes. The other
// success code is refused like any failure, so a status check copied from a
// pull into a push, or the reverse, fails here. Every refusal wraps
// ErrNuvioRequestFailed, which is what makes the API answer 502.
func TestClientSuccessStatus(t *testing.T) {
	tests := []struct {
		clientCall
		success int
	}{
		{listProfiles, http.StatusOK},
		{listAddons, http.StatusOK},
		{pullCollections, http.StatusOK},
		{avatarImages, http.StatusOK},
		{pushAddons, http.StatusNoContent},
		{pushCollections, http.StatusNoContent},
		{pullHomeOrder, http.StatusOK},
		{pushHomeOrder, http.StatusNoContent},
	}
	statuses := []int{
		http.StatusOK, http.StatusNoContent, http.StatusBadRequest,
		http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError,
	}

	for _, tc := range tests {
		for _, status := range statuses {
			t.Run(fmt.Sprintf("%s %d", tc.name, status), func(t *testing.T) {
				c, _ := fakeNuvioServer(t, status, "[]")
				err := tc.call(c)
				if status == tc.success {
					if err != nil {
						t.Fatalf("err = %v, want nil", err)
					}
					return
				}
				if !errors.Is(err, ErrNuvioRequestFailed) {
					t.Fatalf("err = %v, want it to wrap ErrNuvioRequestFailed", err)
				}
			})
		}
	}
}

// TestClientUpstreamFailures covers the failures that aren't a status: a
// pull whose 200 body isn't the JSON it expects, and Nuvio not answering at
// all. Both wrap ErrNuvioRequestFailed.
func TestClientUpstreamFailures(t *testing.T) {
	for _, call := range []clientCall{listProfiles, listAddons, pullCollections, pullHomeOrder} {
		t.Run(call.name+" malformed body", func(t *testing.T) {
			c, _ := fakeNuvioServer(t, http.StatusOK, `{"not":"an array"`)
			if err := call.call(c); !errors.Is(err, ErrNuvioRequestFailed) {
				t.Fatalf("err = %v, want it to wrap ErrNuvioRequestFailed", err)
			}
		})
	}

	for _, call := range []clientCall{listProfiles, listAddons, pullCollections, pushAddons, pushCollections, pullHomeOrder, pushHomeOrder} {
		t.Run(call.name+" unreachable", func(t *testing.T) {
			srv := httptest.NewServer(http.NotFoundHandler())
			srv.Close()
			if err := call.call(NewClient(srv.URL, "publishable-key")); !errors.Is(err, ErrNuvioRequestFailed) {
				t.Fatalf("err = %v, want it to wrap ErrNuvioRequestFailed", err)
			}
		})
	}
}

// TestListProfilesDecodes reads the fields Uno uses and ignores the rest; a
// null avatar field reads as "". An empty or null list comes back as an
// empty, non-nil slice, which the API writes as [] rather than null.
func TestListProfilesDecodes(t *testing.T) {
	c, _ := fakeNuvioServer(t, http.StatusOK,
		`[{"id":"p-1","user_id":"u-1","profile_index":2,"name":"Kids","uses_primary_addons":true,`+
			`"avatar_color_hex":"#1E88E5","avatar_id":null,"avatar_url":"https://img.example/k.png",`+
			`"pin_enabled":true,"created_at":"2026-01-01T00:00:00Z"}]`)
	profiles, err := c.ListProfiles(context.Background(), "access-token")
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	want := []NuvioProfile{{
		ID: "p-1", UserID: "u-1", ProfileIndex: 2, Name: "Kids", UsesPrimaryAddons: true,
		AvatarColorHex: "#1E88E5", AvatarURL: "https://img.example/k.png", PinEnabled: true,
	}}
	if !reflect.DeepEqual(profiles, want) {
		t.Fatalf("profiles = %+v, want %+v", profiles, want)
	}

	for _, body := range []string{"[]", "null"} {
		c, _ := fakeNuvioServer(t, http.StatusOK, body)
		profiles, err := c.ListProfiles(context.Background(), "access-token")
		if err != nil {
			t.Fatalf("ListProfiles(%s): %v", body, err)
		}
		if profiles == nil || len(profiles) != 0 {
			t.Fatalf("ListProfiles(%s) = %#v, want an empty non-nil slice", body, profiles)
		}
	}
}

func TestListAddonsDecodes(t *testing.T) {
	c, _ := fakeNuvioServer(t, http.StatusOK,
		`[{"url":"https://a.example/manifest.json","name":"A","enabled":false,"sort_order":4}]`)
	addons, err := c.ListAddons(context.Background(), "access-token", 3)
	if err != nil {
		t.Fatalf("ListAddons: %v", err)
	}
	want := []NuvioAddon{{URL: "https://a.example/manifest.json", Name: "A", Enabled: false, SortOrder: 4}}
	if !reflect.DeepEqual(addons, want) {
		t.Fatalf("addons = %+v, want %+v", addons, want)
	}
}

// TestPullCollectionsUnwrapsTheBlob covers the RPC's envelope: a profile
// that never had collections pushed is an empty array, and a row's
// collections come back byte-for-byte as Nuvio sent them, since push sends
// every collection Uno doesn't own back unchanged.
func TestPullCollectionsUnwrapsTheBlob(t *testing.T) {
	c, _ := fakeNuvioServer(t, http.StatusOK, `[]`)
	pulled, err := c.PullCollections(context.Background(), "access-token", 3)
	if err != nil {
		t.Fatalf("PullCollections: %v", err)
	}
	if len(pulled) != 0 {
		t.Fatalf("pulled = %s, want none", pulled)
	}

	const foreign = `{"id":"foreign", "n":12345678901234567890,"x":[1.50]}`
	c, _ = fakeNuvioServer(t, http.StatusOK,
		`[{"profile_id":3,"collections_json":[`+foreign+`,{"id":"b"}],"updated_at":"2026-09-25T00:00:00Z"}]`)
	pulled, err = c.PullCollections(context.Background(), "access-token", 3)
	if err != nil {
		t.Fatalf("PullCollections: %v", err)
	}
	if len(pulled) != 2 || string(pulled[0]) != foreign || string(pulled[1]) != `{"id":"b"}` {
		t.Fatalf("pulled = %s, want [%s {\"id\":\"b\"}]", pulled, foreign)
	}
}

// TestPullHomeOrderUnwrapsTheList covers the RPC's envelope: a profile whose
// list was never saved has no row, and a null list is none either. A list
// comes back byte-for-byte as Nuvio sent it, since push sends every row Uno
// doesn't manage back unchanged, and one sent as a JSON string comes back as
// the object it holds.
func TestPullHomeOrderUnwrapsTheList(t *testing.T) {
	const list = `{"items":[{"order":0, "n":12345678901234567890}],"show_catalog_type":false}`
	for _, tc := range []struct {
		name, body, want string
	}{
		{"no row", `[]`, ""},
		{"null", `[{"settings_json":null}]`, ""},
		{"object", `[{"profile_id":3,"platform":"home_catalog_shared","settings_json":` + list + `}]`, list},
		{"string", `[{"settings_json":` + strconv.Quote(list) + `}]`, list},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := fakeNuvioServer(t, http.StatusOK, tc.body)
			pulled, err := c.PullHomeOrder(context.Background(), "access-token", 3)
			if err != nil {
				t.Fatalf("PullHomeOrder: %v", err)
			}
			if string(pulled) != tc.want {
				t.Fatalf("pulled = %s, want %s", pulled, tc.want)
			}
		})
	}
}

// TestAvatarImagesBuildsStorageURLs turns each built-in avatar's storage_path
// into its image in Nuvio's public "avatars" bucket, as Nuvio's apps do, and
// leaves out an avatar with no path.
func TestAvatarImagesBuildsStorageURLs(t *testing.T) {
	c, _ := fakeNuvioServer(t, http.StatusOK,
		`[{"id":"avatar_lalo","storage_path":"animals/bram-v1.png","bg_color":"#F45392"},`+
			`{"id":"slash","storage_path":"/x.png"},{"id":"none","storage_path":""}]`)
	images, err := c.AvatarImages(context.Background(), "access-token")
	if err != nil {
		t.Fatalf("AvatarImages: %v", err)
	}
	base := c.baseURL + "/storage/v1/object/public/avatars/"
	want := map[string]string{"avatar_lalo": base + "animals/bram-v1.png", "slash": base + "x.png"}
	if !reflect.DeepEqual(images, want) {
		t.Fatalf("images = %v, want %v", images, want)
	}
}
