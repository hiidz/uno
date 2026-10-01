import type { ReactNode } from 'react'
import type { Catalog, Collection } from '@/api'
import type { GenreLookups } from '@/features/library/useLibrary'
import { CatalogBlock } from './CatalogBlock'
import { SavedCatalogPreview, SavedCollectionPreview } from './SavedPreviews'

/** What a catalog holds: its spec tiles beside one page of its results. `lead`
 *  sits above the tiles. */
export function CatalogBody({ catalog, genres, lead }: { catalog: Catalog; genres: GenreLookups; lead?: ReactNode }) {
  return (
    <div className="ed-container">
      <div className="ed ed-results">
        <div className="ed-form flex flex-col gap-4">
          {lead}
          <div className="fold-detail">
            <CatalogBlock catalog={catalog} genres={genres} />
          </div>
        </div>
        <SavedCatalogPreview key={catalog.params} type={catalog.type} params={catalog.params} />
      </div>
    </div>
  )
}

/** What a collection holds: a card for each folder, its catalogs as blocks
 *  that open in place, beside the Preview panel. `lead` sits above the cards. */
export function CollectionBody({
  collection,
  genres,
  lead,
}: {
  collection: Collection
  genres: GenreLookups
  lead?: ReactNode
}) {
  const byID = new Map((collection.catalogs ?? []).map((c) => [c.id, c]))
  return (
    <div className="ed-container">
      <div className="ed ed-preview">
        <div className="ed-form flex flex-col gap-3">
          {lead}
          {(collection.folders ?? []).map((folder) => (
            <div key={folder.id} className="fold-detail">
              <span className="flex items-center gap-2 text-[15px] font-bold">
                {folder.cover_emoji && <span aria-hidden="true">{folder.cover_emoji}</span>}
                {folder.title}
              </span>
              <div className="mt-2">
                {(folder.refs ?? []).flatMap((ref, index) => {
                  const catalog = byID.get(ref.catalog_id)
                  if (!catalog) return []
                  return [
                    <CatalogBlock
                      key={`${catalog.id}::${ref.genre}::${index}`}
                      catalog={catalog}
                      genres={genres}
                      narrowedTo={ref.genre}
                      foldable
                    />,
                  ]
                })}
              </div>
            </div>
          ))}
        </div>
        <SavedCollectionPreview collection={collection} />
      </div>
    </div>
  )
}
