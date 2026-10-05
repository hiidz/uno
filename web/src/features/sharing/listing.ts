import type { Catalog } from '@/api'
import { recipeLine } from '@/features/library/recipe'
import type { GenreLookups } from '@/features/library/useLibrary'
import type { CatalogListItem } from './CatalogList'
import type { SharingSticker } from './sharingState'

/** A catalog as a `CatalogList` row: its name over its recipe line. */
export function catalogItem(catalog: Catalog, genres: GenreLookups, sticker?: SharingSticker): CatalogListItem {
  return {
    key: catalog.id,
    name: catalog.name,
    line: recipeLine(catalog, catalog.type === 'movie' ? genres.movie : genres.tv),
    sticker,
  }
}
