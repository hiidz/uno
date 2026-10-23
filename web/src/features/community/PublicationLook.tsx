import type { ReactNode } from 'react'
import type { Collection, WithheldAddress } from '@/api'
import { VIEW_MODE_LABELS } from '@/features/collections/collectionForm'
import { FocusGIFTile, LeftOutPicture, PictureTile, type LeftOutShape } from '@/features/preview/art'
import { shapeClass } from '@/features/preview/heroArt'
import { MarkLine } from '@/features/sharing/MarkViews'
import { OpenTile, SetTile, TILE } from '@/features/sharing/MarkedTiles'
import { folderMarkID, folderMarkWords, type UpdateMarks } from '@/features/sharing/updateMarks'
import { pluralCount } from '@/lib/plural'

type Folder = NonNullable<Collection['folders']>[number]
type PictureField = WithheldAddress['field']

/** Where each address the server left out was: its address by the folder's
 *  key (`''` for the background image) and the field, `''` for none. */
export function withheldLookup(withheld: readonly WithheldAddress[]): (key: string, field: PictureField) => string {
  const byPlace = new Map(withheld.map((entry) => [`${entry.key}/${entry.field}`, entry.url]))
  return (key, field) => byPlace.get(`${key}/${field}`) ?? ''
}

/** What a folder picture shows on the page: Off for a focus GIF turned off,
 *  Left out with its address when the server left it out, the picture when
 *  set, else None. */
export type FolderPicture =
  | { state: 'off' | 'none' }
  | { state: 'out'; url: string }
  | { state: 'set'; url: string }

export function folderPicture(folder: Folder, field: Exclude<PictureField, 'backdrop_image_url' | 'cover_image_url'>, leftOut: string): FolderPicture {
  if (field === 'focus_gif_url' && !folder.focus_gif_enabled) return { state: 'off' }
  if (leftOut) return { state: 'out', url: leftOut }
  const url = folder[field]
  return url ? { state: 'set', url } : { state: 'none' }
}

/**
 * A collection publication's Appearance side, beside the same Preview panel
 * as Filters: the collection's own settings straight under the switch, its
 * background image beside them only when it is set or left out; then a
 * Folders heading over a card per folder. Each card shows the folder's tile
 * as Nuvio draws it (cover, else emoji, else name) beside its tile shape and
 * name setting, then each picture set, at its shape. A picture the server
 * left out shows as its box with the address to copy (`LeftOutPicture`), so
 * nothing on this side loads it. A folder whose art changed in a waiting
 * update carries the mark Filters gives it, under the same id, so the In this
 * update shelf scrolls to it on either side.
 */
export function PublicationLook({
  collection,
  withheld,
  marks,
}: {
  collection: Collection
  withheld: readonly WithheldAddress[]
  marks: UpdateMarks | null
}) {
  const leftOut = withheldLookup(withheld)
  const folders = collection.folders ?? []
  return (
    <>
      <CollectionSettings collection={collection} leftOut={leftOut('', 'backdrop_image_url')} />
      <div className="border-b-[1.5px] mt-3 flex items-baseline justify-between gap-3 border-[color-mix(in_srgb,var(--accent)_45%,transparent)] pb-1.5">
        <h3 className="type-sign text-sign m-0 text-[var(--accent)]">Folders</h3>
        <span className="type-data text-dim text-meta" aria-label={pluralCount(folders.length, 'folder')}>
          {folders.length}
        </span>
      </div>
      {folders.map((folder) => (
        <FolderLook key={folder.id} folder={folder} leftOut={leftOut} words={marks ? folderMarkWords(marks.folders.get(folder.id)) : ''} />
      ))}
    </>
  )
}

/** How folders open and the focus glow, stacked, with the background image
 *  beside them when it is set or left out; nothing in its place otherwise. */
function CollectionSettings({ collection, leftOut }: { collection: Collection; leftOut: string }) {
  const backdrop = leftOut ? (
    <LeftOutPicture url={leftOut} shape="BACKDROP" />
  ) : (
    collection.backdrop_image_url && (
      <span className="look-backdrop">
        <PictureTile src={collection.backdrop_image_url} shape="LANDSCAPE" />
      </span>
    )
  )
  return (
    <div className="flex items-stretch gap-2">
      <dl className="m-0 grid min-w-0 flex-1 auto-rows-fr gap-2">
        <SetTile fact={{ label: 'Folder pages', value: folderPages(collection) }} className={`${TILE} bg-raised`} change={undefined} />
        <SetTile fact={{ label: 'Focus glow', value: collection.focus_glow_enabled ? 'On' : 'Off' }} className={`${TILE} bg-raised`} change={undefined} />
      </dl>
      {backdrop && <Picture label="Background image">{backdrop}</Picture>}
    </div>
  )
}

/** How a folder opens, as the editor names it, and its All tab where Tabbed
 *  Grids gives it one. */
function folderPages(collection: Collection): string {
  const label = VIEW_MODE_LABELS[collection.view_mode]
  return collection.view_mode === 'TABBED_GRID' && collection.show_all_tab ? `${label} · All tab` : label
}

const PICTURES = [
  { field: 'focus_gif_url', label: 'Focus GIF' },
  { field: 'hero_backdrop_url', label: 'Hero backdrop' },
  { field: 'title_logo_url', label: 'Title logo' },
] as const

/** One folder's card: its heading as on Filters, its tile beside its tile
 *  shape and name setting and any picture that is off or unset, then each
 *  picture set or left out, two to a row from `sm` up. */
function FolderLook({ folder, leftOut, words }: { folder: Folder; leftOut: ReturnType<typeof withheldLookup>; words: string }) {
  const pictures = PICTURES.map(({ field, label }) => ({ label, field, picture: folderPicture(folder, field, leftOut(folder.id, field)) }))
  const shown = pictures.filter(({ picture }) => picture.state === 'set' || picture.state === 'out')
  const unset = pictures.filter(({ picture }) => picture.state === 'off' || picture.state === 'none')
  return (
    <div id={folderMarkID(folder.id)} tabIndex={-1} className="fold-detail outline-none">
      <div className="border-line-hi mb-4 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 border-b pb-3">
        <span className="text-title flex min-w-0 items-center gap-2">
          {folder.cover_emoji && <span aria-hidden="true">{folder.cover_emoji}</span>}
          <span className="min-w-0 [overflow-wrap:anywhere]">{folder.title}</span>
        </span>
        <span className="type-data text-dim text-meta shrink-0">{pluralCount(folder.refs?.length ?? 0, 'catalog')}</span>
        <MarkLine words={words} className="order-3 basis-full" />
      </div>
      <div className="grid items-end gap-3.5 sm:grid-cols-[auto_minmax(0,1fr)]">
        <FolderTile folder={folder} leftOut={leftOut(folder.id, 'cover_image_url')} />
        <dl className="m-0 grid grid-cols-[repeat(auto-fill,minmax(124px,1fr))] gap-2">
          <SetTile fact={{ label: 'Tile shape', value: shapeName(folder) }} className={`${TILE} bg-ground`} change={undefined} />
          <SetTile fact={{ label: 'Name on tile', value: folder.hide_title ? 'Hidden' : 'Shown' }} className={`${TILE} bg-ground`} change={undefined} />
          {unset.map(({ label, picture }) => (
            <OpenTile key={label} fact={{ label, value: picture.state === 'off' ? 'Off' : 'None' }} change={undefined} />
          ))}
        </dl>
      </div>
      {shown.length > 0 && (
        <div className="mt-4 grid items-start gap-4 sm:grid-cols-2">
          {shown.map(({ label, field, picture }) => (
            <Picture key={label} label={label}>
              <FolderPictureView folder={folder} field={field} picture={picture} />
            </Picture>
          ))}
        </div>
      )}
    </div>
  )
}

function shapeName(folder: Folder): string {
  return folder.tile_shape.charAt(0) + folder.tile_shape.slice(1).toLowerCase()
}

/** The folder's tile: its cover, or the cover's box when the server left it
 *  out (Nuvio would draw the cover, not the emoji); with no cover, the emoji,
 *  else the folder's name, labelled Tile. */
function FolderTile({ folder, leftOut }: { folder: Folder; leftOut: string }) {
  if (leftOut) {
    return (
      <Picture label="Cover">
        <LeftOutPicture url={leftOut} shape={folder.tile_shape} />
      </Picture>
    )
  }
  if (folder.cover_image_url) {
    return (
      <Picture label="Cover">
        <PictureTile src={folder.cover_image_url} shape={folder.tile_shape} />
      </Picture>
    )
  }
  return (
    <Picture label="Tile">
      <span className={`art-pic ${shapeClass(folder.tile_shape)}`}>
        {folder.cover_emoji ? (
          <span className="art-pic-emoji">{folder.cover_emoji}</span>
        ) : (
          <span className="art-pic-note text-meta font-semibold [overflow-wrap:anywhere]">{folder.title}</span>
        )}
      </span>
    </Picture>
  )
}

const OUT_SHAPES: Record<(typeof PICTURES)[number]['field'], (folder: Folder) => LeftOutShape> = {
  focus_gif_url: (folder) => folder.tile_shape,
  hero_backdrop_url: () => 'LANDSCAPE',
  title_logo_url: () => 'LOGO',
}

/** A picture set or left out: the focus GIF at the tile shape, the hero
 *  backdrop at 16:9, the title logo at its own proportions. */
function FolderPictureView({ folder, field, picture }: { folder: Folder; field: (typeof PICTURES)[number]['field']; picture: FolderPicture }) {
  if (picture.state === 'out') return <LeftOutPicture url={picture.url} shape={OUT_SHAPES[field](folder)} />
  if (picture.state !== 'set') return null
  if (field === 'focus_gif_url') return <FocusGIFTile src={picture.url} shape={folder.tile_shape} />
  if (field === 'hero_backdrop_url') return <PictureTile src={picture.url} shape="LANDSCAPE" />
  return <PictureTile src={picture.url} />
}

/** A picture straight on its card, a small dim label above it. */
function Picture({ label, children }: { label: string; children: ReactNode }) {
  return (
    <figure className="m-0 flex min-w-0 flex-col gap-1.5">
      <figcaption className="text-dim text-caption">{label}</figcaption>
      {children}
    </figure>
  )
}
