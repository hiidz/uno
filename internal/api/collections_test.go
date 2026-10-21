package api

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/vault"
)

// validateInlineCatalogs checks every recipe a collection save carries — the
// folders' New specs and the catalog edits alike — and replaces each one's
// params with the canonical form its row will store. One bad recipe of
// either kind is a 400, and so is one carrying a key its type doesn't
// declare.
func TestValidateInlineCatalogs(t *testing.T) {
	s := newProfileTestServer(t, newTestVaultDB(t))
	const clean = `{"vote_count_gte":0,"sort_by":"popularity.desc"}`
	const broken = `{"sort_by":"bogus.desc"}`
	const undeclared = `{"primary_release_year":2020}`

	form := func(newParams, editParams string) vault.CollectionForm {
		return vault.CollectionForm{
			Title: "C", ViewMode: "TABBED_GRID",
			Folders: []vault.FolderData{{FolderArt: vault.FolderArt{TileShape: "POSTER"}, Title: "F", Catalogs: []vault.FolderCatalogRef{
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
	const want = `{"sort_by":"popularity.desc"}`
	if got := ok.Folders[0].Catalogs[0].New.Params; got != want {
		t.Errorf("new spec params = %s, want %s", got, want)
	}
	if got := ok.CatalogEdits[0].Params; got != want {
		t.Errorf("catalog edit params = %s, want %s", got, want)
	}

	for _, tc := range []struct {
		name string
		form vault.CollectionForm
	}{
		{"broken new spec", form(broken, clean)},
		{"broken catalog edit", form(clean, broken)},
		{"undeclared key in a new spec", form(undeclared, clean)},
		{"series key in a movie catalog edit", form(clean, `{"with_networks":"213"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.validateInlineCatalogs(t.Context(), &tc.form); !errors.Is(err, vault.ErrInvalidInput) {
				t.Fatalf("err = %v, want it to wrap vault.ErrInvalidInput", err)
			}
		})
	}
}
