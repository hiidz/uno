import type { BuilderMode, CatalogFormState } from '@/features/catalogs/catalogForm'
import type { CollectionFormState } from '@/features/collections/collectionForm'

/**
 * What the builder's right pane is editing, when it isn't showing home.
 *
 * One target at a time, and no way to express two: the pane has a single
 * occupant, so selecting another row in the rail replaces this rather than
 * adding to it. That's why there is no tab strip — there is nothing to tab
 * between.
 */
export type EditorTarget =
  | {
      kind: 'catalog'
      mode: BuilderMode
      initial: CatalogFormState
      /** Present only when editing; a create or duplicate has no row yet. */
      catalogID?: string
      /** The library row this was opened from, so the rail can mark it
       *  selected. Absent for a brand-new item, which has no row. */
      sourceID?: string
    }
  | {
      kind: 'collection'
      mode: BuilderMode
      initial: CollectionFormState
      droppedRefs: string[]
      collectionID?: string
      sourceID?: string
    }
