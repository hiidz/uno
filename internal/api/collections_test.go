package api

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

// validateInlineCatalogs checks every recipe a collection save carries — the
// folders' New specs and the catalog edits alike — and stamps each with the
// fingerprint its row will store. One bad recipe of either kind is a 400.
func TestValidateInlineCatalogs(t *testing.T) {
	s := newProfileTestServer(t, newTestVaultDB(t))
	const clean = `{"sort_by":"popularity.desc"}`
	const broken = `{"sort_by":"bogus.desc"}`

	form := func(newParams, editParams string) vault.CollectionForm {
		return vault.CollectionForm{
			Title: "C",
			Folders: []vault.FolderData{{Title: "F", Catalogs: []vault.FolderCatalogRef{
				{New: &vault.NewScopedCatalog{Key: "draft:a", Type: "movie", Name: "New", Provider: "tmdb", Params: newParams}},
			}}},
			CatalogEdits: []vault.ScopedCatalogEdit{
				{ID: uuid.New(), Type: "movie", Provider: "tmdb", Name: "Edited", Params: editParams},
			},
		}
	}

	ok := form(clean, clean)
	if err := s.validateInlineCatalogs(t.Context(), &ok); err != nil {
		t.Fatalf("clean recipes: %v", err)
	}
	want, err := provider.Fingerprint("movie", "tmdb", clean)
	if err != nil {
		t.Fatalf("fingerprinting the clean recipe: %v", err)
	}
	if got := ok.Folders[0].Catalogs[0].New.Fingerprint; got != want {
		t.Errorf("new spec fingerprint = %q, want %q", got, want)
	}
	if got := ok.CatalogEdits[0].Fingerprint; got != want {
		t.Errorf("catalog edit fingerprint = %q, want %q", got, want)
	}

	for _, tc := range []struct {
		name string
		form vault.CollectionForm
	}{
		{"broken new spec", form(broken, clean)},
		{"broken catalog edit", form(clean, broken)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.validateInlineCatalogs(t.Context(), &tc.form); !errors.Is(err, vault.ErrInvalidInput) {
				t.Fatalf("err = %v, want it to wrap vault.ErrInvalidInput", err)
			}
		})
	}
}
