package vault

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/vault/migrations"
)

// A folder ref's genre goes out as its catalogSources entry's "genre"; an
// unfiltered ref has no "genre" key at all rather than an empty one. One
// catalog referenced under two genres becomes two sources, and a ref whose
// catalog the tree lacks is skipped.
func TestPushPayloadCarriesRefGenre(t *testing.T) {
	filtered := Catalog{ID: uuid.New(), Type: "movie", Provider: "tmdb"}
	plain := Catalog{ID: uuid.New(), Type: "series", Provider: "tmdb"}
	tree := CollectionWithFolders{
		Collection: Collection{ID: uuid.New(), Title: "C"},
		Folders: []FolderWithCatalogs{{
			Folder: Folder{ID: uuid.New(), Title: "F"},
			Refs: []FolderRef{
				{CatalogID: filtered.ID, Genre: "Western"},
				{CatalogID: plain.ID},
				{CatalogID: uuid.New(), Genre: "Gone"},
				{CatalogID: filtered.ID, Genre: "War"},
			},
		}},
		Catalogs: []Catalog{filtered, plain},
	}

	raw, err := json.Marshal(tree.PushPayload().Folders[0].CatalogSources)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var sources []map[string]any
	if err := json.Unmarshal(raw, &sources); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(sources) != 3 {
		t.Fatalf("got %d sources, want 3: %s", len(sources), raw)
	}
	if sources[0]["genre"] != "Western" || sources[2]["genre"] != "War" {
		t.Errorf("filtered source genres = %v, %v, want Western, War: %s", sources[0]["genre"], sources[2]["genre"], raw)
	}
	if sources[0]["catalogId"] != "tmdb-"+filtered.ID.String() || sources[0]["catalogId"] != sources[2]["catalogId"] {
		t.Errorf("the two genres of one catalog carry catalogIds %v and %v, want both tmdb-%s", sources[0]["catalogId"], sources[2]["catalogId"], filtered.ID)
	}
	if _, ok := sources[1]["genre"]; ok || sources[1]["addonId"] != AddonID {
		t.Errorf("unfiltered source = %v, want no genre key and Uno's addon id: %s", sources[1], raw)
	}
}

// Nuvio reads an absent focusGlowEnabled/focusGifEnabled as true, so both go
// out as an explicit false when off. The appearance URLs are omitted when
// empty, like coverImageUrl.
func TestPushPayloadAppearanceFields(t *testing.T) {
	tree := CollectionWithFolders{
		Collection: Collection{ID: uuid.New(), Title: "C"},
		Folders: []FolderWithCatalogs{
			{Folder: Folder{ID: uuid.New(), Title: "Off"}},
			{Folder: Folder{
				ID: uuid.New(), Title: "On",
				FocusGIFURL: "https://example.com/f.gif", FocusGIFEnabled: true,
				HeroBackdropURL: "https://example.com/b.jpg", HeroVideoURL: "https://example.com/v.mp4",
				TitleLogoURL: "https://example.com/l.png",
			}},
		},
	}

	raw, err := tree.PushJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var pushed struct {
		FocusGlowEnabled *bool            `json:"focusGlowEnabled"`
		Folders          []map[string]any `json:"folders"`
	}
	if err := json.Unmarshal(raw, &pushed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if pushed.FocusGlowEnabled == nil || *pushed.FocusGlowEnabled {
		t.Errorf("focusGlowEnabled = %v, want explicit false: %s", pushed.FocusGlowEnabled, raw)
	}
	off, on := pushed.Folders[0], pushed.Folders[1]
	if v, ok := off["focusGifEnabled"]; !ok || v != false {
		t.Errorf("off folder focusGifEnabled = %v (present %v), want explicit false: %s", v, ok, raw)
	}
	for _, key := range []string{"focusGifUrl", "heroBackdropUrl", "heroVideoUrl", "titleLogoUrl"} {
		if _, ok := off[key]; ok {
			t.Errorf("off folder carries empty %s: %s", key, raw)
		}
	}
	want := map[string]any{
		"focusGifEnabled": true,
		"focusGifUrl":     "https://example.com/f.gif",
		"heroBackdropUrl": "https://example.com/b.jpg",
		"heroVideoUrl":    "https://example.com/v.mp4",
		"titleLogoUrl":    "https://example.com/l.png",
	}
	for key, v := range want {
		if on[key] != v {
			t.Errorf("on folder %s = %v, want %v: %s", key, on[key], v, raw)
		}
	}
}

// v4PushFixture is a database at schema version 4 holding the collections
// the pushed_hash migration's copy of the push payload must hash exactly as
// the vault's does: a title needing escapes, an empty folder list, a folder
// with every optional field empty and one with every one set, false bools,
// genres empty and set, folders and refs stored out of order, and an
// off-Home collection. A, B and C are as last pushed; D changed since and E
// never was.
const v4PushFixture = `
INSERT INTO profiles (id, token, nuvio_user_id, nuvio_profile_index, nuvio_profile_uuid)
VALUES ('aaaaaaaa-0000-4000-8000-000000000001', 't1', 'u1', 1, 'n1');
INSERT INTO recipes (hash, type, provider, params, created_at) VALUES
    ('r-movie', 'movie', 'tmdb', '{}', '2026-09-30T00:00:00Z'),
    ('r-series', 'series', 'tmdb', '{}', '2026-09-30T00:00:00Z');
INSERT INTO catalogs (id, name, recipe_hash, owner_id, collection_id, home_sort_order, show_in_home, created_at, updated_at) VALUES
    ('cacacaca-0000-4000-8000-000000000001', 'Movies', 'r-movie', 'aaaaaaaa-0000-4000-8000-000000000001', NULL, NULL, 1, '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z'),
    ('cacacaca-0000-4000-8000-000000000002', 'Series', 'r-series', 'aaaaaaaa-0000-4000-8000-000000000001', NULL, NULL, 1, '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z');
INSERT INTO collections (id, title, owner_id, pin_to_top, view_mode, show_all_tab, backdrop_image_url, focus_glow_enabled,
                         home_sort_order, version, pushed_version, created_at, updated_at) VALUES
    ('cccccccc-0000-4000-8000-00000000000a', 'A & <Night>', 'aaaaaaaa-0000-4000-8000-000000000001', 1, 'ROWS', 0, 'https://example.com/b.jpg', 0, 0, 3, 3, '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z'),
    ('cccccccc-0000-4000-8000-00000000000b', '<B>', 'aaaaaaaa-0000-4000-8000-000000000001', 0, 'TABBED_GRID', 1, '', 1, 1, 1, 1, '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z'),
    ('cccccccc-0000-4000-8000-00000000000c', 'Off Home', 'aaaaaaaa-0000-4000-8000-000000000001', 0, 'ROWS', 0, '', 1, NULL, 2, 2, '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z'),
    ('cccccccc-0000-4000-8000-00000000000d', 'Changed', 'aaaaaaaa-0000-4000-8000-000000000001', 0, 'ROWS', 0, '', 1, 2, 2, 1, '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z'),
    ('cccccccc-0000-4000-8000-00000000000e', 'Never pushed', 'aaaaaaaa-0000-4000-8000-000000000001', 0, 'ROWS', 0, '', 1, NULL, 1, NULL, '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z');
INSERT INTO folders (id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url, focus_gif_url,
                     focus_gif_enabled, hero_video_url, hero_backdrop_url, title_logo_url) VALUES
    ('ffffffff-0000-4000-8000-000000000002', 'cccccccc-0000-4000-8000-00000000000a', 'Full & <Set>', 1, 'POSTER', 1, '🎃',
     'https://example.com/c.jpg', 'https://example.com/f.gif', 1, 'https://example.com/v.mp4', 'https://example.com/h.jpg', 'https://example.com/l.png'),
    ('ffffffff-0000-4000-8000-000000000001', 'cccccccc-0000-4000-8000-00000000000a', 'Bare', 0, 'LANDSCAPE', 0, '', '', '', 0, '', '', ''),
    ('ffffffff-0000-4000-8000-000000000003', 'cccccccc-0000-4000-8000-00000000000c', 'F', 0, 'SQUARE', 0, '', '', '', 1, '', '', '');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES
    ('ffffffff-0000-4000-8000-000000000002', 'cacacaca-0000-4000-8000-000000000001', 2, ''),
    ('ffffffff-0000-4000-8000-000000000002', 'cacacaca-0000-4000-8000-000000000002', 0, 'Sci-Fi & Fantasy'),
    ('ffffffff-0000-4000-8000-000000000002', 'cacacaca-0000-4000-8000-000000000001', 1, 'Action'),
    ('ffffffff-0000-4000-8000-000000000003', 'cacacaca-0000-4000-8000-000000000002', 0, '');
`

// The pushed_hash migration's frozen copy of the push payload hashes every
// collection it backfills exactly as the vault's live one does, byte for
// byte, over every case v4PushFixture holds; the dry run's push-hash check
// says so.
func TestFrozenPushHashesMatchTheLiveBuilder(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "vault.db")
	if err := migrate(ctx, path, migrations.All()[:4]); err != nil {
		t.Fatalf("migrating to version 4: %v", err)
	}
	d, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, v4PushFixture); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}

	report, err := DryRun(ctx, path, liveChecks)
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if report.PushHashesBackfilled != 3 || report.PushHashesPending != 2 || len(report.PushHashMismatches) != 0 {
		t.Errorf("push hashes = %d backfilled, %d pending, mismatches %q; want 3, 2 and none",
			report.PushHashesBackfilled, report.PushHashesPending, report.PushHashMismatches)
	}
}

// A backfilled collection whose stored hash its live payload doesn't give
// is named, by id and title.
func TestPushHashMismatchesNamesADrift(t *testing.T) {
	tree := CollectionWithFolders{Collection: Collection{ID: uuid.New(), Title: "Drifted"}}
	raw, err := tree.PushJSON()
	if err != nil {
		t.Fatal(err)
	}
	agreeing := tree
	agreeing.pushedHash = PushHash(raw)
	drifted := tree
	drifted.pushedHash = "stale"

	got := pushHashMismatches([]CollectionWithFolders{agreeing, drifted})
	if want := []string{"collection " + tree.ID.String() + " (Drifted)"}; !slices.Equal(got, want) {
		t.Errorf("mismatches = %q, want %q", got, want)
	}
}
