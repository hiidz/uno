import type { Catalog, Folder } from '@/api'
import { recipeLine } from '@/features/library/recipe'
import type { GenreLookups } from '@/features/library/useLibrary'
import type { CatalogListItem } from './CatalogList'

/** A catalog as a `CatalogList` row: its name over its recipe line. */
export function catalogItem(catalog: Catalog, genres: GenreLookups, sticker?: string): CatalogListItem {
  return {
    key: catalog.id,
    name: catalog.name,
    line: recipeLine(catalog, catalog.type === 'movie' ? genres.movie : genres.tv),
    sticker,
  }
}

/** A folder's refs as `CatalogList` rows, in order, each narrowed ref's
 *  genre after its recipe line ("Most popular · Horror • Slasher"). A ref
 *  whose catalog `catalogs` doesn't hold is left out. */
export function folderItems(
  folder: Pick<Folder, 'refs'>,
  catalogs: ReadonlyMap<string, Catalog>,
  genres: GenreLookups,
): CatalogListItem[] {
  return (folder.refs ?? []).flatMap((ref) => {
    const catalog = catalogs.get(ref.catalog_id)
    if (!catalog) return []
    const item = catalogItem(catalog, genres)
    return [{ ...item, key: `${catalog.id}::${ref.genre}`, line: ref.genre ? `${item.line} • ${ref.genre}` : item.line }]
  })
}
