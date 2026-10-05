package api

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/hiidz/uno/internal/nuvio"
)

// recordingVerifier authenticates every token as "real:"+token and records
// each one it is asked to verify.
type recordingVerifier struct{ tokens []string }

func (v *recordingVerifier) Verify(_ context.Context, token string) (nuvio.Claims, error) {
	v.tokens = append(v.tokens, token)
	return nuvio.Claims{Sub: "real:" + token}, nil
}

// Only the exact bypass token authenticates as the fake account; every
// other token, a prefix or an extension of it included, goes to the real
// verifier.
func TestDevBypassVerifier(t *testing.T) {
	next := &recordingVerifier{}
	v := NewDevBypassVerifier(next, "dev-secret")

	claims, err := v.Verify(t.Context(), "dev-secret")
	if err != nil || claims.Sub != DevBypassSub {
		t.Fatalf("Verify(bypass token) = %+v, %v; want sub %q", claims, err, DevBypassSub)
	}
	if len(next.tokens) != 0 {
		t.Fatalf("the bypass token reached the real verifier: %v", next.tokens)
	}

	others := []string{"dev-secre", "dev-secret2", "DEV-SECRET", "a-real-jwt"}
	for _, token := range others {
		claims, err := v.Verify(t.Context(), token)
		if err != nil || claims.Sub != "real:"+token {
			t.Errorf("Verify(%q) = %+v, %v; want the real verifier's answer", token, claims, err)
		}
	}
	if !reflect.DeepEqual(next.tokens, others) {
		t.Fatalf("real verifier saw %v, want %v", next.tokens, others)
	}
}

// The bypass token is served from a per-profile in-memory store and never
// reaches Nuvio, an index the fake account doesn't hold is refused rather
// than aliased to a real one, and every other token goes to the real client.
func TestDevBypassNuvio(t *testing.T) {
	upstream := &fakeNuvio{profiles: []nuvio.NuvioProfile{{ID: "real", UserID: "real-user", ProfileIndex: 1}}}
	n := NewDevBypassNuvio(upstream, "dev-secret")
	ctx := t.Context()

	t.Run("profiles", func(t *testing.T) {
		profiles, err := n.ListProfiles(ctx, "dev-secret")
		if err != nil || len(profiles) != 2 || profiles[0].UserID != DevBypassSub || profiles[1].ProfileIndex != 2 {
			t.Fatalf("bypass profiles = %+v, %v; want the two fake ones", profiles, err)
		}
		profiles, err = n.ListProfiles(ctx, "a-real-jwt")
		if err != nil || !reflect.DeepEqual(profiles, upstream.profiles) {
			t.Fatalf("real-token profiles = %+v, %v; want the real client's", profiles, err)
		}
	})

	t.Run("avatar images", func(t *testing.T) {
		upstream.avatarImages = map[string]string{"avatar_lalo": "https://nuvio.example/a.png"}
		images, err := n.AvatarImages(ctx, "dev-secret")
		if err != nil || len(images) != 0 || upstream.avatarCalls != 0 {
			t.Fatalf("bypass avatar images = %v, %v (upstream asked %d times); want none, without Nuvio", images, err, upstream.avatarCalls)
		}
		images, err = n.AvatarImages(ctx, "a-real-jwt")
		if err != nil || !reflect.DeepEqual(images, upstream.avatarImages) {
			t.Fatalf("real-token avatar images = %v, %v; want the real client's", images, err)
		}
	})

	t.Run("each fake profile stores its own push", func(t *testing.T) {
		addons := []nuvio.NuvioAddon{{URL: "https://uno.example/u/t/manifest.json", Name: "Uno Catalog", Enabled: true}}
		if err := n.PushAddons(ctx, "dev-secret", 1, addons); err != nil {
			t.Fatalf("PushAddons: %v", err)
		}
		addons[0].Name = "changed after the push"
		collections := []json.RawMessage{json.RawMessage(`{"id":"c"}`)}
		if err := n.PushCollections(ctx, "dev-secret", 2, collections); err != nil {
			t.Fatalf("PushCollections: %v", err)
		}

		got, err := n.ListAddons(ctx, "dev-secret", 1)
		if err != nil || len(got) != 1 || got[0].Name != "Uno Catalog" {
			t.Fatalf("profile 1 addons = %+v, %v; want the one pushed, as pushed", got, err)
		}
		if got, _ := n.ListAddons(ctx, "dev-secret", 2); len(got) != 0 {
			t.Fatalf("profile 2 addons = %+v, want none", got)
		}
		pulled, err := n.PullCollections(ctx, "dev-secret", 2)
		if err != nil || !reflect.DeepEqual(pulled, collections) {
			t.Fatalf("profile 2 collections = %s, %v; want %s", pulled, err, collections)
		}
		if pulled, _ := n.PullCollections(ctx, "dev-secret", 1); len(pulled) != 0 {
			t.Fatalf("profile 1 collections = %s, want none", pulled)
		}
		homeOrder := json.RawMessage(`{"items":[]}`)
		if err := n.PushHomeOrder(ctx, "dev-secret", 2, homeOrder); err != nil {
			t.Fatalf("PushHomeOrder: %v", err)
		}
		if pulled, err := n.PullHomeOrder(ctx, "dev-secret", 2); err != nil || string(pulled) != string(homeOrder) {
			t.Fatalf("profile 2 home order = %s, %v; want %s", pulled, err, homeOrder)
		}
		if pulled, _ := n.PullHomeOrder(ctx, "dev-secret", 1); pulled != nil {
			t.Fatalf("profile 1 home order = %s, want none", pulled)
		}
		if len(upstream.pushAddonsCalls) != 0 || len(upstream.pushCollectionsCalls) != 0 || len(upstream.pushHomeOrderCalls) != 0 {
			t.Fatal("a bypass push reached the real client")
		}
	})

	t.Run("unseeded index", func(t *testing.T) {
		calls := map[string]func() error{
			"ListAddons":      func() error { _, err := n.ListAddons(ctx, "dev-secret", 3); return err },
			"PushAddons":      func() error { return n.PushAddons(ctx, "dev-secret", 3, nil) },
			"PullCollections": func() error { _, err := n.PullCollections(ctx, "dev-secret", 3); return err },
			"PushCollections": func() error { return n.PushCollections(ctx, "dev-secret", 3, nil) },
			"PullHomeOrder":   func() error { _, err := n.PullHomeOrder(ctx, "dev-secret", 3); return err },
			"PushHomeOrder":   func() error { return n.PushHomeOrder(ctx, "dev-secret", 3, nil) },
		}
		for name, call := range calls {
			if err := call(); !errors.Is(err, errDevBypassUnknownProfile) {
				t.Errorf("%s at index 3: err = %v, want errDevBypassUnknownProfile", name, err)
			}
		}
	})

	t.Run("real token", func(t *testing.T) {
		if err := n.PushAddons(ctx, "a-real-jwt", 1, nil); err != nil {
			t.Fatalf("PushAddons: %v", err)
		}
		if err := n.PushCollections(ctx, "a-real-jwt", 1, nil); err != nil {
			t.Fatalf("PushCollections: %v", err)
		}
		upstream.homeOrder = json.RawMessage(`{"items":[]}`)
		if pulled, err := n.PullHomeOrder(ctx, "a-real-jwt", 1); err != nil || string(pulled) != `{"items":[]}` {
			t.Fatalf("real-token home order = %s, %v; want the real client's", pulled, err)
		}
		if err := n.PushHomeOrder(ctx, "a-real-jwt", 1, nil); err != nil {
			t.Fatalf("PushHomeOrder: %v", err)
		}
		if len(upstream.pushAddonsCalls) != 1 || len(upstream.pushCollectionsCalls) != 1 || len(upstream.pushHomeOrderCalls) != 1 {
			t.Fatalf("real client pushes = %d addons, %d collections, %d home order; want 1 each",
				len(upstream.pushAddonsCalls), len(upstream.pushCollectionsCalls), len(upstream.pushHomeOrderCalls))
		}
	})
}
