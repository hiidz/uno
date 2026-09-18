import type { Catalog } from '@/api'
import type { CatalogFormState } from '@/features/catalogs/catalogForm'
import type { CollectionFormState } from '@/features/collections/collectionForm'

/**
 * What the builder's right pane is editing, when it isn't showing home.
 *
 * One target at a time, and no way to express two: the pane has a single
 * occupant, so selecting another row in the rail replaces this rather than
 * adding to it. That's why there is no tab strip — there is nothing to tab
 * between.
 *
 * Neither variant has a `mode`/`duplicate` any more: duplicating either a
 * catalog (`useCatalogMutations`'s `create`, called directly with a
 * duplicate payload) or a collection (`useCollectionMutations`'s
 * `duplicate`) is a single atomic server call, not a pre-filled form the
 * editor saves to create the copy, so every row this pane opens is always
 * editing a real one.
 */
export type EditorTarget =
  | {
      kind: 'catalog'
      initial: CatalogFormState
      catalogID: string
      /** The library row this was opened from, so the rail can mark it
       *  selected. Always equal to `catalogID` today — kept as a separate
       *  field for symmetry with the collection variant below. */
      sourceID?: string
    }
  | {
      kind: 'collection'
      initial: CollectionFormState
      collectionID?: string
      sourceID?: string
      /** Every catalog this collection's folders reference, listed or
       *  scoped, straight from the row that produced `initial` — set when
       *  that row might not be in the library yet (a collection just
       *  duplicated, whose scoped copies a stale `library.collections` read
       *  wouldn't have). Falls back to a library lookup by `sourceID` when
       *  absent; see `Workspace.tsx`'s `editingCollectionCatalogs`. */
      initialCatalogs?: Catalog[]
    }
