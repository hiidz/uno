// The push payload: one of Uno's own collections in the camelCase shape
// Nuvio's collections blob holds, which push sends and the push record keeps
// (pushrecord.go). A collection needs a push when what push would send for it
// now differs from what its owner's last push sent.

package vault

import (
	"encoding/json"

	"github.com/google/uuid"
)

// AddonID is Uno's addon id, constant across every profile — identity in
// the addon protocol comes from the URL path (/u/{token}/...), never from
// this id. Every pushed source carries it, and the addon's manifest declares
// it (addon.ID).
const AddonID = "hiidz.uno.catalog"

// ManifestID is the manifest-facing id for one catalog: provider-prefixed so
// it's the same string that round-trips as Nuvio's collections
// catalogSources[].catalogId — see the "Push wire shape" section of
// docs/data-model.md and the sample in
// docs/api/samples/collections-basic.json. The addon's manifest and the push
// both use it (addon.ManifestID).
func ManifestID(c Catalog) string {
	return c.Provider + "-" + c.ID.String()
}

// CatalogSource is one folder catalogSources[] entry — a reference to one of
// this addon's catalogs, by addon+type+catalog id. Built against the field
// name the public doc documents; real pulled data uses "sources" for the
// containing array name but the entry shape itself is unaffected. Genre,
// when set, is a name from the catalog's manifest genre extra; Nuvio sends it
// back as that extra when it loads the folder's row.
type CatalogSource struct {
	AddonID   string `json:"addonId"`
	Type      string `json:"type"`
	CatalogID string `json:"catalogId"`
	Genre     string `json:"genre,omitempty"`
}

// PushFolder is one collection folder in push/pull's camelCase wire shape —
// distinct from Folder, which is snake_case and DB-column-shaped.
// FocusGIFEnabled has no omitempty: Nuvio reads an absent key as true, so
// false has to be sent to mean off.
type PushFolder struct {
	ID              string          `json:"id"`
	Title           string          `json:"title"`
	CoverImageURL   string          `json:"coverImageUrl,omitempty"`
	CoverEmoji      string          `json:"coverEmoji,omitempty"`
	FocusGIFURL     string          `json:"focusGifUrl,omitempty"`
	FocusGIFEnabled bool            `json:"focusGifEnabled"`
	HeroBackdropURL string          `json:"heroBackdropUrl,omitempty"`
	HeroVideoURL    string          `json:"heroVideoUrl,omitempty"`
	TitleLogoURL    string          `json:"titleLogoUrl,omitempty"`
	TileShape       string          `json:"tileShape"`
	HideTitle       bool            `json:"hideTitle"`
	CatalogSources  []CatalogSource `json:"catalogSources"`
}

// PushCollection is one collection in push/pull's camelCase wire shape.
// FocusGlowEnabled has no omitempty for the same reason as
// PushFolder.FocusGIFEnabled.
type PushCollection struct {
	ID               string       `json:"id"`
	Title            string       `json:"title"`
	BackdropImageURL string       `json:"backdropImageUrl,omitempty"`
	PinToTop         bool         `json:"pinToTop"`
	FocusGlowEnabled bool         `json:"focusGlowEnabled"`
	ViewMode         string       `json:"viewMode"`
	ShowAllTab       bool         `json:"showAllTab"`
	Folders          []PushFolder `json:"folders"`
}

// PushPayload is tree as push sends it: each folder's refs resolved through
// tree.Catalogs into addonId/type/catalogId triples, each carrying its
// reference's genre when one is set — so a catalog referenced under two
// genres becomes two sources. A ref whose catalog tree.Catalogs lacks is
// skipped rather than failing the whole push.
func (tree CollectionWithFolders) PushPayload() PushCollection {
	catalogs := make(map[uuid.UUID]Catalog, len(tree.Catalogs))
	for _, c := range tree.Catalogs {
		catalogs[c.ID] = c
	}
	folders := make([]PushFolder, len(tree.Folders))
	for i, f := range tree.Folders {
		folders[i] = pushFolder(f, catalogs)
	}
	return PushCollection{
		ID:               tree.ID.String(),
		Title:            tree.Title,
		BackdropImageURL: tree.BackdropImageURL,
		PinToTop:         tree.PinToTop,
		FocusGlowEnabled: tree.FocusGlowEnabled,
		ViewMode:         tree.ViewMode,
		ShowAllTab:       tree.ShowAllTab,
		Folders:          folders,
	}
}

// pushFolder is folder f as push sends it, its refs resolved through
// catalogs.
func pushFolder(f FolderWithCatalogs, catalogs map[uuid.UUID]Catalog) PushFolder {
	return PushFolder{
		ID:              f.ID.String(),
		Title:           f.Title,
		CoverImageURL:   f.CoverImageURL,
		CoverEmoji:      f.CoverEmoji,
		FocusGIFURL:     f.FocusGIFURL,
		FocusGIFEnabled: f.FocusGIFEnabled,
		HeroBackdropURL: f.HeroBackdropURL,
		HeroVideoURL:    f.HeroVideoURL,
		TitleLogoURL:    f.TitleLogoURL,
		TileShape:       f.TileShape,
		HideTitle:       f.HideTitle,
		CatalogSources:  pushSources(f.Refs, catalogs),
	}
}

// pushSources is refs as a folder's catalogSources, skipping any ref whose
// catalog catalogs lacks.
func pushSources(refs []FolderRef, catalogs map[uuid.UUID]Catalog) []CatalogSource {
	sources := make([]CatalogSource, 0, len(refs))
	for _, ref := range refs {
		if catalog, ok := catalogs[ref.CatalogID]; ok {
			sources = append(sources, CatalogSource{
				AddonID:   AddonID,
				Type:      catalog.Type,
				CatalogID: ManifestID(catalog),
				Genre:     ref.Genre,
			})
		}
	}
	return sources
}

// PushJSON is tree's push payload as the exact bytes push sends for it.
func (tree CollectionWithFolders) PushJSON() ([]byte, error) {
	return json.Marshal(tree.PushPayload())
}
