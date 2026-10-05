import type { Catalog, CatalogPayload } from '@/api'
import { useToast, type ToastMessage } from '@/components/useToast'
import type { RefOption } from './refs'

/** Writes a library catalog, settling once the server has it. */
export type CopyToLibrary = (payload: CatalogPayload) => Promise<unknown>

const COPIED: ToastMessage = { text: 'Copied into your library', tone: 'success' }

/**
 * A folder row's Copy into library: a new library catalog with the name, type
 * and recipe of the row's catalog as this editor holds it, staged edits and
 * drafts included, written at once rather than with the collection's Save.
 * Offered only for a catalog that lives in this collection (`available`); the
 * outcome is a message (`toast`) for the row to show beside its menu.
 */
export function useCopyToLibrary(option: RefOption | undefined, copyToLibrary: CopyToLibrary) {
  const [toast, setToast] = useToast()
  const catalog = scopedCatalog(option)

  function copy() {
    if (!catalog) return
    copyToLibrary(libraryPayload(catalog)).then(
      () => setToast(COPIED),
      (err: unknown) => setToast({ text: `Couldn't copy: ${(err as Error).message}`, tone: 'danger' }),
    )
  }

  return { available: catalog !== undefined, copy, toast }
}

function scopedCatalog(option: RefOption | undefined): Catalog | undefined {
  if (!option || option.catalog.collection_id === null) return undefined
  return option.catalog
}

function libraryPayload(catalog: Catalog): CatalogPayload {
  return { type: catalog.type, name: catalog.name, provider: catalog.provider, params: catalog.params, collection_id: null }
}
