/**
 * Merges the owned and community responses for one resource into a single
 * dataset, tagging every row with ownership.
 *
 * The merge is not optional. `GET /api/catalogs` and `GET /api/collections`
 * are both `is_public = TRUE` with **no owner exclusion**, so a public row you
 * own comes back in *both* responses. Without dedup it appears twice; without
 * owned-wins it renders as someone else's and loses its edit/delete
 * affordances.
 *
 * Ownership is membership in the owned response rather than an `owner_id`
 * comparison: `GET /api/p/{i}/catalogs` *is* `owner_id = profile`, so the set
 * is exact by definition and the page never needs the profile's UUID.
 *
 * Order is owned-first, then community in server order — the rail groups by
 * ownership implicitly without a separate sort.
 */
export function mergeOwned<T extends { id: string }>(
  owned: T[],
  community: T[],
): (T & { owned: boolean })[] {
  const ownedIds = new Set(owned.map((item) => item.id))
  const merged: (T & { owned: boolean })[] = owned.map((item) => ({ ...item, owned: true }))
  for (const item of community) {
    if (!ownedIds.has(item.id)) merged.push({ ...item, owned: false })
  }
  return merged
}
