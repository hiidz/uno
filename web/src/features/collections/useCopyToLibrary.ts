import type { Catalog, CatalogPayload } from '@/api'
import { useToasts, type ToastInput } from '@/components/toastQueue'
import type { RefOption } from './refs'

/** Writes a library catalog, settling once the server has it. */
export type CopyToLibrary = (payload: CatalogPayload) => Promise<unknown>

const COPIED: ToastInput = { text: 'Copied into your library', tone: 'success' }

/**
 * A folder row's Copy into library: a new library catalog with the name, type
 * and recipe of the row's catalog as this editor holds it, staged edits and
 * drafts included, written at once rather than with the collection's Save.
 * Offered only for a catalog that lives in this collection (`available`); the
 * outcome is a toast.
 */
export function useCopyToLibrary(option: RefOption | undefined, copyToLibrary: CopyToLibrary) {
  const toasts = useToasts()
  const catalog = scopedCatalog(option)

  function copy() {
    if (!catalog) return
    copyToLibrary(libraryPayload(catalog)).then(
      () => toasts.show(COPIED),
      (err: unknown) => toasts.show({ tone: 'danger', text: "Couldn't copy into your library", detail: (err as Error).message }),
    )
  }

  return { available: catalog !== undefined, copy }
}

function scopedCatalog(option: RefOption | undefined): Catalog | undefined {
  if (!option || option.catalog.collection_id === null) return undefined
  return option.catalog
}

function libraryPayload(catalog: Catalog): CatalogPayload {
  return { type: catalog.type, name: catalog.name, provider: catalog.provider, params: catalog.params }
}
