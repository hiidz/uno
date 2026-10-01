package vault

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
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
