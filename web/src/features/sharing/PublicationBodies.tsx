import type { ReactNode } from 'react'
import type { Catalog, Collection } from '@/api'
import type { GenreLookups } from '@/features/library/useLibrary'
import { pluralCount } from '@/lib/plural'
import { CatalogBlock } from './CatalogBlock'
import { SavedCatalogPreview, SavedCollectionPreview } from './SavedPreviews'

type Folder = NonNullable<Collection['folders']>[number]

/** What a catalog holds: its spec tiles beside one page of its results. `lead`
 *  sits above the tiles. A page still loading its publication draws the lead
 *  alone, with no `catalog` yet. */
export function CatalogBody({ catalog, genres, lead }: { catalog?: Catalog; genres: GenreLookups; lead?: ReactNode }) {
  return (
    <div className="ed-container">
      <div className="ed ed-results">
        <div className="ed-form flex flex-col gap-4">
          {lead}
          {catalog && <CatalogBlock catalog={catalog} genres={genres} />}
        </div>
        {catalog && <SavedCatalogPreview key={catalog.params} type={catalog.type} params={catalog.params} />}
      </div>
    </div>
  )
}

/** What a collection holds: a card for each folder, its catalogs as blocks
 *  that open in place, beside the Preview panel. `lead` sits above the cards.
 *  A page still loading its publication draws the lead alone, with no
 *  `collection` yet. */
export function CollectionBody({
  collection,
  genres,
  lead,
}: {
  collection?: Collection
  genres: GenreLookups
  lead?: ReactNode
}) {
  const byID = catalogsByID(collection)
  return (
    <div className="ed-container">
      <div className="ed ed-preview">
        <div className="ed-form flex flex-col gap-3">
          {lead}
          {foldersOf(collection).map((folder) => (
            <FolderCard key={folder.id} folder={folder} catalogs={byID} genres={genres} />
          ))}
        </div>
        {collection && <SavedCollectionPreview collection={collection} />}
      </div>
    </div>
  )
}

function foldersOf(collection: Collection | undefined): Folder[] {
  return collection?.folders ?? []
}

function catalogsByID(collection: Collection | undefined): Map<string, Catalog> {
  return new Map((collection?.catalogs ?? []).map((catalog) => [catalog.id, catalog]))
}

/** One folder: its name with how many catalogs it holds at the end, then those
 *  catalogs as blocks that open in place. */
function FolderCard({
  folder,
  catalogs,
  genres,
}: {
  folder: Folder
  catalogs: ReadonlyMap<string, Catalog>
  genres: GenreLookups
}) {
  const refs = folder.refs ?? []
  return (
    <div className="fold-detail pb-3">
      <div className="flex items-baseline justify-between gap-3">
        <span className="flex min-w-0 items-center gap-2 text-[15px] font-bold">
          {folder.cover_emoji && <span aria-hidden="true">{folder.cover_emoji}</span>}
          {folder.title}
        </span>
        <span className="type-data text-dim shrink-0 text-[12.5px]">{pluralCount(refs.length, 'catalog')}</span>
      </div>
      <div className="mt-2">
        {refs.map((ref, index) => {
          const key = `${ref.catalog_id}::${ref.genre}::${index}`
          return <FolderEntry key={key} catalog={catalogs.get(ref.catalog_id)} genre={ref.genre} genres={genres} />
        })}
      </div>
    </div>
  )
}

/** A folder's catalog as a folded block; nothing for a ref the tree does not
 *  hold. */
function FolderEntry({ catalog, genre, genres }: { catalog: Catalog | undefined; genre: string; genres: GenreLookups }) {
  if (!catalog) return null
  return <CatalogBlock catalog={catalog} genres={genres} narrowedTo={genre} foldable />
}
