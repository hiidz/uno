import { useEffect, useMemo } from 'react'
import type { Catalog, CatalogType, Collection } from '@/api'
import { RecipePreview } from '@/features/catalogs/RecipePreview'
import { CollectionPreview } from '@/features/collections/CollectionPreview'
import { toPreviewCollection } from '@/features/home/preview'
import { useRecipeTiles } from '@/features/preview/useRecipeTiles'

/** `RecipePreview` over a saved recipe nobody is editing here — a subscribed
 *  copy's, or a publication's. Nothing is being typed, so it runs as soon as
 *  it mounts, and Run again still fetches a shuffled recipe's next page. A
 *  saved recipe is one the server already accepted, so never `invalid`. */
export function SavedCatalogPreview({ type, params }: { type: CatalogType; params: string }) {
  const preview = useRecipeTiles(type, params)
  // Callers key it by its params, so it mounts fresh for each recipe and
  // running on mount is running once per recipe; `preview.run` itself isn't
  // stable across renders.
  // oxlint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => preview.run(), [])
  return <RecipePreview preview={preview} type={type} invalid={false} onRun={preview.run} readOnly />
}

/** The collection editor's Preview panel over a saved tree nobody is editing
 *  here, its refs resolved against the tree's own `catalogs`, so every folder
 *  resolves whatever the library holds. */
export function SavedCollectionPreview({ collection }: { collection: Collection }) {
  const preview = useMemo(
    () =>
      toPreviewCollection(
        collection.id,
        collection,
        new Map((collection.catalogs ?? []).map((catalog: Catalog) => [catalog.id, catalog])),
      ),
    [collection],
  )
  return <CollectionPreview collection={preview} />
}
