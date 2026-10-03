import type { Collection } from '@/api'
import type { HomeCollectionEntry } from '@/features/home/pending'

/**
 * Why Push can't run as Home stands, which the builder says in place of
 * letting it fail. The server refuses both too (`refusePush`), so a tab that
 * missed one still can't push it.
 *
 * - `shares-addons`: in Nuvio, the profile uses profile 1's addons, so Uno's
 *   addon pushed to it would show nowhere.
 * - `empty-collection`: a collection on Home has no folders, which Nuvio's
 *   phone and desktop apps leave off Home and Nuvio TV has no guard against.
 */
export type PushBlock = { kind: 'shares-addons' } | { kind: 'empty-collection'; title: string }

/** What blocks a push of `collections`, the Home selection's, read through
 *  `collectionById`: the profile's addon sharing first, since it blocks
 *  whatever Home holds, then the first collection on Home with no folders. A
 *  collection not loaded yet blocks nothing. */
export function pushBlock(
  sharesAddons: boolean,
  collections: readonly HomeCollectionEntry[],
  collectionById: ReadonlyMap<string, Collection>,
): PushBlock | null {
  if (sharesAddons) return { kind: 'shares-addons' }
  for (const entry of collections) {
    const collection = collectionById.get(entry.id)
    if (collection && hasNoFolders(collection)) return { kind: 'empty-collection', title: collection.title }
  }
  return null
}

function hasNoFolders(collection: Collection): boolean {
  return (collection.folders ?? []).length === 0
}
