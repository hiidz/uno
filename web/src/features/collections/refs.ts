import { tmdbKind } from '@/api'
import { catalogSearchText, describeRecipe } from '@/features/library/recipe'
import type { GenreLookups, LibraryCatalog } from '@/features/library/useLibrary'

/**
 * The catalog side of the collection builder: what a folder can reference, and
 * how each reference reads on screen.
 *
 * Derived once per dataset rather than per keystroke: `describeRecipe` parses a
 * catalog's params JSON, and the picker filters on every character typed.
 */

export interface RefOption {
  id: string
  name: string
  catalog: LibraryCatalog
  /** The rendered recipe, joined. The picker shows no artwork, so the recipe is
   *  what distinguishes one catalog from another. */
  recipe: string
  /** Lowercased name + recipe, so a search for "horror" finds a catalog
   *  filtered to that genre and not only one named for it. */
  searchText: string
}

export function buildRefOptions(
  catalogs: LibraryCatalog[],
  genres: GenreLookups,
): RefOption[] {
  return catalogs.map((catalog) => {
    const lookup = genres[tmdbKind(catalog.type)]
    return {
      id: catalog.id,
      name: catalog.name,
      catalog,
      recipe: describeRecipe(catalog, lookup).join(' · ') || 'no filters',
      searchText: catalogSearchText(catalog, lookup),
    }
  })
}

export function indexRefOptions(options: RefOption[]): ReadonlyMap<string, RefOption> {
  return new Map(options.map((option) => [option.id, option]))
}

/**
 * The ids a folder is allowed to reference — the library, which is
 * `owned ∪ is_public` and therefore exactly the server's
 * `owner_id = ? OR is_public = 1`. Handed to `validateCollectionForm` as the
 * mirror of `validateCatalogAccess`.
 */
export function accessibleIDs(options: RefOption[]): ReadonlySet<string> {
  return new Set(options.map((option) => option.id))
}

/** Filters the picker. `exclude` is the ids already in *this* folder: a repeat
 *  inside one folder violates `PRIMARY KEY (folder_id, catalog_id)` and fails
 *  as a 500, so the picker omits them rather than catching it after the click.
 *  The same catalog in a *different* folder stays offered — the schema allows
 *  it. */
export function filterRefOptions(
  options: RefOption[],
  query: string,
  exclude: ReadonlySet<string>,
): RefOption[] {
  const needle = query.trim().toLowerCase()
  return options.filter(
    (option) => !exclude.has(option.id) && (!needle || option.searchText.includes(needle)),
  )
}
