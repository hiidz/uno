import type { ReactNode } from 'react'
import type { Catalog, Collection } from '@/api'
import type { GenreLookups } from '@/features/library/useLibrary'
import { pluralCount } from '@/lib/plural'
import { CatalogBlock } from './CatalogBlock'
import { MarkLine, RemovedCatalogs, RemovedFolders } from './MarkViews'
import { SavedCatalogPreview, SavedCollectionPreview } from './SavedPreviews'
import {
  firstUses,
  folderEntryMark,
  folderMarkID,
  folderMarkWords,
  pageBlockMark,
  type UpdateMarks,
} from './updateMarks'

type Folder = NonNullable<Collection['folders']>[number]

/** What a catalog holds: its spec tiles beside one page of its results. `lead`
 *  sits above the tiles. A page still loading its publication draws the lead
 *  alone, with no `catalog` yet. While an update waits, `marks` edges the
 *  tiles it changes. */
export function CatalogBody({ catalog, genres, lead, marks = null }: CatalogBodyProps) {
  return (
    <div className="ed-container">
      <div className="ed ed-results">
        <div className="ed-form flex flex-col gap-4">
          {lead}
          {catalog && <CatalogBlock catalog={catalog} genres={genres} mark={pageBlockMark(marks, catalog.id)} />}
        </div>
        {catalog && <SavedCatalogPreview key={catalog.params} type={catalog.type} params={catalog.params} />}
      </div>
    </div>
  )
}

/** What a collection holds: a card for each folder, its catalogs as blocks
 *  that open in place, beside the Preview panel. `lead` sits above the cards.
 *  A page still loading its publication draws the lead alone, with no
 *  `collection` yet. While an update waits, `marks` says on each card and
 *  block what it changes, and the folders it removes follow the cards. */
export function CollectionBody({ collection, genres, lead, marks = null }: CollectionBodyProps) {
  const byID = catalogsByID(collection)
  const firsts = firstUses(foldersOf(collection))
  return (
    <div className="ed-container">
      <div className="ed ed-preview">
        <div className="ed-form flex flex-col gap-3">
          {lead}
          {foldersOf(collection).map((folder) => (
            <FolderCard key={folder.id} folder={folder} catalogs={byID} genres={genres} marks={marks} firsts={firsts} />
          ))}
          <RemovedFolders names={removedFolders(marks)} />
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

/** One folder: its name with how many catalogs it holds at the end, ruled off
 *  from those catalogs as blocks that open in place; while an update waits,
 *  what it does to the folder under the name and the catalogs it removes at
 *  the foot. */
function FolderCard({ folder, catalogs, genres, marks, firsts }: FolderCardProps) {
  const refs = folder.refs ?? []
  return (
    <div id={folderMarkID(folder.id)} tabIndex={-1} className="fold-detail pb-3 outline-none">
      <div className="border-line-hi mb-1 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 border-b pb-3">
        <span className="flex min-w-0 items-center gap-2 text-[16.5px] font-bold">
          {folder.cover_emoji && <span aria-hidden="true">{folder.cover_emoji}</span>}
          <span className="min-w-0 [overflow-wrap:anywhere]">{folder.title}</span>
        </span>
        <span className="type-data text-dim shrink-0 text-[12.5px]">{pluralCount(refs.length, 'catalog')}</span>
        <MarkLine words={folderWords(marks, folder.id)} className="order-3 basis-full" />
      </div>
      <div className="mt-1">
        {refs.map((ref, index) => {
          const key = `${ref.catalog_id}::${ref.genre}::${index}`
          const mark = folderEntryMark(marks, folder.id, ref.catalog_id, firsts.has(`${folder.id}/${index}`))
          return <FolderEntry key={key} catalog={catalogs.get(ref.catalog_id)} genre={ref.genre} genres={genres} mark={mark} />
        })}
        <RemovedCatalogs names={removedFrom(marks, folder.id)} />
      </div>
    </div>
  )
}

interface CatalogBodyProps {
  catalog?: Catalog
  genres: GenreLookups
  lead?: ReactNode
  marks?: UpdateMarks | null
}

interface CollectionBodyProps {
  collection?: Collection
  genres: GenreLookups
  lead?: ReactNode
  marks?: UpdateMarks | null
}

interface FolderCardProps {
  folder: Folder
  catalogs: ReadonlyMap<string, Catalog>
  genres: GenreLookups
  marks: UpdateMarks | null
  /** `folderKey/index` of each catalog's first ref in the new version. */
  firsts: ReadonlySet<string>
}

function folderWords(marks: UpdateMarks | null, key: string): string {
  return marks ? folderMarkWords(marks.folders.get(key)) : ''
}

function removedFrom(marks: UpdateMarks | null, key: string): readonly string[] {
  return marks?.removed.get(key) ?? []
}

function removedFolders(marks: UpdateMarks | null): readonly string[] {
  return marks?.removedFolders ?? []
}

/** A folder's catalog as a folded block; nothing for a ref the tree does not
 *  hold. */
function FolderEntry({ catalog, genre, genres, mark }: FolderEntryProps) {
  if (!catalog) return null
  return <CatalogBlock key={blockKey(mark)} catalog={catalog} genres={genres} narrowedTo={genre} foldable mark={mark} />
}

/** A block whose mark opens it mounts again when the mark arrives, which the
 *  update's changes may do after the page's details have drawn it closed. */
function blockKey(mark: ReturnType<typeof folderEntryMark>): string {
  return mark?.startOpen ? 'opened' : 'closed'
}

interface FolderEntryProps {
  catalog: Catalog | undefined
  genre: string
  genres: GenreLookups
  mark: ReturnType<typeof folderEntryMark>
}
