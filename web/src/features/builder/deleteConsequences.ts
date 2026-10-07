import type { Catalog, Collection } from '@/api'
import { isPublished } from '@/features/sharing/sharingState'
import { pluralCount } from '@/lib/plural'

/** What deleting a row takes with it, beyond the row itself. Each field says
 *  nothing unless it applies, so a confirm never states what isn't true. */
export interface DeleteConsequences {
  /** "Also removes it from: Community · 2 collections · Nuvio (next push)". */
  removedFrom: string | null
  /** People who added a published row keep their copies. */
  addersKeep: boolean
  /** Catalogs made only for a collection, which go with it. */
  scopedCatalogs: number
}

function removedFrom(places: string[]): string | null {
  return places.length > 0 ? `Also removes it from: ${places.join(' · ')}` : null
}

function usesCatalog(collection: Collection, catalogID: string): boolean {
  return (collection.folders ?? []).some((folder) =>
    (folder.refs ?? []).some((ref) => ref.catalog_id === catalogID),
  )
}

/** A catalog's consequences: Community when published, each collection whose
 *  folders use it, and Nuvio when it, or a collection using it, is on Home as
 *  last pushed. */
export function catalogDeleteConsequences(
  catalog: Catalog,
  collections: Collection[],
): DeleteConsequences {
  const using = collections.filter((collection) => usesCatalog(collection, catalog.id))
  const onNuvio =
    catalog.home_position !== null || using.some((c) => c.home_position !== null)
  const places: string[] = []
  if (isPublished(catalog)) places.push('Community')
  if (using.length > 0) places.push(pluralCount(using.length, 'collection'))
  if (onNuvio) places.push('Nuvio (next push)')
  return { removedFrom: removedFrom(places), addersKeep: isPublished(catalog), scopedCatalogs: 0 }
}

/** A collection's consequences: Community when published, Nuvio when it is on
 *  Home as last pushed, and the catalogs scoped to it. */
export function collectionDeleteConsequences(collection: Collection): DeleteConsequences {
  const places: string[] = []
  if (isPublished(collection)) places.push('Community')
  if (collection.home_position !== null) places.push('Nuvio (next push)')
  const scopedCatalogs = (collection.catalogs ?? []).filter(
    (c) => c.collection_id === collection.id,
  ).length
  return { removedFrom: removedFrom(places), addersKeep: isPublished(collection), scopedCatalogs }
}

/** How many of the collection's own catalogs go with it. */
export function scopedCatalogsLine(count: number): string {
  return `${pluralCount(count, 'catalog')} made only for this collection ${count === 1 ? 'goes' : 'go'} with it.`
}
