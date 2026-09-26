import type { Catalog } from '@/api'
import type { CatalogFormState } from '@/features/catalogs/catalogForm'
import type { CollectionFormState } from '@/features/collections/collectionForm'

/**
 * What the builder's right pane is editing, when it isn't showing home.
 *
 * Always a saved row: a new catalog or collection is named into existence
 * before its editor opens, and a duplicate is one server call whose finished
 * copy opens like any other row. `id` is that row's, which the rail marks
 * selected.
 *
 * One target at a time, and no way to express two: the pane has a single
 * occupant, so selecting another row in the rail replaces this rather than
 * adding to it. That's why there is no tab strip — there is nothing to tab
 * between.
 */
export type EditorTarget =
  | {
      kind: 'catalog'
      id: string
      initial: CatalogFormState
    }
  | {
      kind: 'collection'
      id: string
      initial: CollectionFormState
      /** Every catalog this collection's folders reference, listed or
       *  scoped, straight from the row that produced `initial` — set when
       *  that row might not be in the library yet (a collection just
       *  duplicated, whose scoped copies a stale `library.collections` read
       *  wouldn't have). Falls back to a library lookup by `id` when absent;
       *  see `Workspace.tsx`'s `editingCollectionCatalogs`. */
      initialCatalogs?: Catalog[]
    }
