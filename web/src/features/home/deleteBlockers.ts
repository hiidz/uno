/**
 * What keeps a row from being deleted: Nuvio may still hold it. A catalog or
 * collection on Home, or a catalog a collection on Home uses, stays until a
 * push has taken it off — the same rule the server enforces in the delete
 * itself (`internal/vault/delete_guard.go`), in the same words, so a Delete
 * disabled here and a 409 from a stale tab read alike.
 */

import type { Collection } from '@/api'

/**
 * Home as the server holds it, which is what the last push sent Nuvio: the
 * catalogs with a Home row of their own, Discover-only included, and the
 * collections on Home, in Home order. The pending Home edits don't count —
 * Nuvio has none of them until they're pushed.
 */
export interface PushedHome {
  catalogIDs: ReadonlySet<string>
  collections: readonly Collection[]
}

export const TAKE_OFF_HOME = 'Take it off Home and push first.'
export const PUSH_FIRST = 'Push first: Nuvio may still show it in a collection.'

/**
 * Why the listed catalog `catalogID` can't be deleted yet, or `null`: it has
 * a Home row of its own; a collection on Home uses it, the first in Home
 * order named; or any collection on Home needs a push, since Nuvio then still
 * holds that collection's last pushed folders, which may use it.
 */
export function catalogDeleteBlocker(catalogID: string, home: PushedHome): string | null {
  if (home.catalogIDs.has(catalogID)) return TAKE_OFF_HOME
  const user = home.collections.find((c) => usesCatalog(c, catalogID))
  if (user) return `Remove it from “${user.title}” and push first.`
  return home.collections.some((c) => c.needs_push) ? PUSH_FIRST : null
}

/** Why the collection `collectionID` can't be deleted yet — it's on Home —
 *  or `null`. */
export function collectionDeleteBlocker(collectionID: string, home: PushedHome): string | null {
  return home.collections.some((c) => c.id === collectionID) ? TAKE_OFF_HOME : null
}

function usesCatalog(collection: Collection, catalogID: string): boolean {
  return (collection.catalogs ?? []).some((c) => c.id === catalogID)
}

/**
 * `PushedHome` from the two selection responses. Each collection is read
 * through `collectionById`, which prefers the library's row, so its
 * `needs_push` and catalogs are as fresh as the last save.
 */
export function pushedHome(
  catalogSelection: readonly { id: string }[] | undefined,
  collectionSelection: readonly Collection[] | undefined,
  collectionById: ReadonlyMap<string, Collection>,
): PushedHome {
  return {
    catalogIDs: new Set((catalogSelection ?? []).map((c) => c.id)),
    collections: (collectionSelection ?? []).map((c) => collectionById.get(c.id) ?? c),
  }
}

/** Why a row can't be deleted yet, by id, or `null` when it can be. */
export interface DeleteBlockers {
  catalog: (id: string) => string | null
  collection: (id: string) => string | null
}

export function deleteBlockersFor(home: PushedHome): DeleteBlockers {
  return {
    catalog: (id) => catalogDeleteBlocker(id, home),
    collection: (id) => collectionDeleteBlocker(id, home),
  }
}

/** A Delete button's own props for the row `name`: disabled, with the
 *  reason as its title, while `reason` holds the row. */
export function deleteButton(name: string, reason: string | null): { label: string; title: string; disabled: boolean } {
  const label = `Delete ${name}`
  return { label, title: reason ?? label, disabled: reason !== null }
}
