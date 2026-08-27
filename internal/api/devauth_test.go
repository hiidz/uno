package api

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hiidz/uno/internal/nuvio"
)

func TestDevBypassVerifier(t *testing.T) {
	t.Run("bypass token authenticates as DevBypassSub without calling next", func(t *testing.T) {
		next := &fakeVerifier{err: errors.New("next must not be called")}
		v := NewDevBypassVerifier(next, "let-me-in")

		claims, err := v.Verify(context.Background(), "let-me-in")
		if err != nil {
			t.Fatalf("Verify() error = %v", err)
		}
		if claims.Sub != DevBypassSub {
			t.Errorf("Sub = %q, want %q", claims.Sub, DevBypassSub)
		}
		if next.verifyCalled {
			t.Error("next.Verify was called for the bypass token")
		}
	})

	t.Run("any other token falls through to next", func(t *testing.T) {
		next := &fakeVerifier{claims: nuvio.Claims{Sub: "real-user"}}
		v := NewDevBypassVerifier(next, "let-me-in")

		claims, err := v.Verify(context.Background(), "some-real-jwt")
		if err != nil {
			t.Fatalf("Verify() error = %v", err)
		}
		if !next.verifyCalled || next.receivedTok != "some-real-jwt" {
			t.Errorf("next.Verify called = %v with %q, want true with %q", next.verifyCalled, next.receivedTok, "some-real-jwt")
		}
		if claims.Sub != "real-user" {
			t.Errorf("Sub = %q, want %q", claims.Sub, "real-user")
		}
	})
}

func TestDevBypassNuvio(t *testing.T) {
	const bypassToken = "let-me-in"

	t.Run("ListProfiles returns the fake profile for the bypass token", func(t *testing.T) {
		next := &fakeNuvioClient{err: errors.New("next must not be called")}
		c := NewDevBypassNuvio(next, bypassToken)

		profiles, err := c.ListProfiles(context.Background(), bypassToken)
		if err != nil {
			t.Fatalf("ListProfiles() error = %v", err)
		}
		if len(profiles) != 1 || profiles[0].UserID != DevBypassSub {
			t.Errorf("profiles = %+v, want one profile owned by %q", profiles, DevBypassSub)
		}
	})

	t.Run("other tokens fall through to next", func(t *testing.T) {
		next := &fakeNuvioClient{profiles: []nuvio.NuvioProfile{{ID: "real"}}}
		c := NewDevBypassNuvio(next, bypassToken)

		profiles, err := c.ListProfiles(context.Background(), "real-token")
		if err != nil {
			t.Fatalf("ListProfiles() error = %v", err)
		}
		if len(profiles) != 1 || profiles[0].ID != "real" {
			t.Errorf("profiles = %+v, want the real client's list", profiles)
		}
	})

	t.Run("push then list round-trips addons and collections in memory", func(t *testing.T) {
		next := &fakeNuvioClient{}
		c := NewDevBypassNuvio(next, bypassToken)
		ctx := context.Background()

		if err := c.PushAddons(ctx, bypassToken, 1, []nuvio.PushAddonInput{{URL: "u", Name: "n", Enabled: true}}); err != nil {
			t.Fatalf("PushAddons() error = %v", err)
		}
		addons, err := c.ListAddons(ctx, bypassToken, 1)
		if err != nil {
			t.Fatalf("ListAddons() error = %v", err)
		}
		if len(addons) != 1 || addons[0].URL != "u" {
			t.Errorf("addons = %+v, want the pushed addon", addons)
		}

		blob := []json.RawMessage{json.RawMessage(`{"id":"c1"}`)}
		if err := c.PushCollections(ctx, bypassToken, 1, blob); err != nil {
			t.Fatalf("PushCollections() error = %v", err)
		}
		got, err := c.PullCollections(ctx, bypassToken, 1)
		if err != nil {
			t.Fatalf("PullCollections() error = %v", err)
		}
		if len(got) != 1 || string(got[0]) != `{"id":"c1"}` {
			t.Errorf("collections = %s, want the pushed blob", got)
		}
	})
}
