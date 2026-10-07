import { tmdbKind } from '@/api'
import { catalogListing } from '@/features/library/recipe'
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
    const { line, searchText } = catalogListing(catalog, genres[tmdbKind(catalog.type)])
    return { id: catalog.id, name: catalog.name, catalog, recipe: line || 'No filters', searchText }
  })
}

export function indexRefOptions(options: RefOption[]): ReadonlyMap<string, RefOption> {
  return new Map(options.map((option) => [option.id, option]))
}

/**
 * The ids a folder is allowed to reference — the library, which under the
 * closed-graph model is exactly this profile's own listed catalogs, the same
 * set the server's `validateFolderRefs` checks against. Handed to
 * `validateCollectionForm` for that check.
 */
export function accessibleIDs(options: RefOption[]): ReadonlySet<string> {
  return new Set(options.map((option) => option.id))
}

/** Whether `option` is a library catalog, linked here rather than a
 *  collection's own. An unavailable one is neither. */
export function isLinked(option: RefOption | undefined): boolean {
  if (!option) return false
  return option.catalog.collection_id === null
}

/** Filters the picker. `exclude` is the catalogs *this* folder already holds
 *  unfiltered: the picker adds an unfiltered ref, and a second one repeats the
 *  (catalog, genre) pair `PRIMARY KEY (folder_id, catalog_id, genre)` forbids.
 *  A catalog whose refs here are all narrowed to a genre stays offered, as does
 *  the same catalog in a *different* folder. */
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
