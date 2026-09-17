import { useMemo } from 'react'
import { CollectionMeta } from '@/features/preview/CollectionMeta'
import {
  normalizeTileShape,
  normalizeViewMode,
  type PreviewCollection,
  type PreviewFolder,
  type PreviewSource,
} from '@/features/preview/model'
import { TILE_ASPECT } from '@/features/preview/tiles'
import type { CollectionFormState } from './collectionForm'
import type { RefOption } from './refs'

/**
 * "On your TV" — DESIGN.md's docked panel, in the catalog editor's results
 * panel's place: a 16:9 frame holding the collection's row from the live
 * draft, **not interactive**. Opening a folder to see its content lives in
 * Preview on TV once the collection is saved, not here — this only has to
 * answer "what will the row itself look like", which is layout, not content,
 * so nothing is fetched to draw it.
 *
 * **Layout, not content.** A collection has no recipe of its own — it's
 * folders of catalog references — so there is nothing to run against TMDB at
 * this level. What it does have is a shape: folders as tiles at their own
 * `tile_shape`. That shape comes entirely from form state, so the row below
 * costs no request and is drawn without asking.
 */
export function CollectionPreview({
  state,
  optionByID,
}: {
  state: CollectionFormState
  optionByID: ReadonlyMap<string, RefOption>
}) {
  const collection = useMemo(() => previewFromForm(state, optionByID), [state, optionByID])

  return (
    <div className="ed-pv">
      <div className="ed-pv-head">
        <span className="type-eyebrow">On your TV</span>
      </div>

      <div className="bg-tv-bezel border-line-hi border p-2.5">
        <div className="bg-tv-screen relative flex aspect-video items-end overflow-hidden p-3">
          {collection.folders.length === 0 ? (
            <p className="type-data text-dimmer m-0 text-[10.5px] leading-[1.45]">
              No folders yet, so this row is empty.
            </p>
          ) : (
            <div className="flex items-end gap-2 overflow-hidden">
              {collection.folders.map((folder) => (
                <TVFolderTile key={folder.id} folder={folder} />
              ))}
            </div>
          )}
        </div>
      </div>

      <p>Folders open in Preview on TV once saved.</p>

      <CollectionMeta collection={collection} />
    </div>
  )
}

/** A folder, drawn the way its tile would sit on the real TV row — cover
 *  image, then emoji, then title, at the folder's own shape — but as a plain
 *  `<span>`, never a button: this panel draws no folder page, so nothing here
 *  is a target to open. */
function TVFolderTile({ folder }: { folder: PreviewFolder }) {
  const height = 76
  const width = height * TILE_ASPECT[folder.tileShape]
  const name = folder.title || 'Untitled folder'

  return (
    <span
      style={{ width: `${width}px`, height: `${height}px` }}
      title={name}
      className="bg-raised border-line relative grid shrink-0 place-items-center overflow-hidden rounded-[2px] border px-1"
    >
      {folder.coverEmoji ? (
        <span aria-hidden="true" className="text-[16px] leading-none">
          {folder.coverEmoji}
        </span>
      ) : (
        !folder.hideTitle && (
          <span aria-hidden="true" className="type-data text-dimmer text-center text-[8px] leading-tight">
            {name}
          </span>
        )
      )}
      {folder.coverImageUrl && (
        <img
          src={folder.coverImageUrl}
          alt=""
          loading="lazy"
          className="absolute inset-0 h-full w-full object-cover"
        />
      )}
    </span>
  )
}

/**
 * The form's own state as a previewable collection.
 *
 * A folder's identity here is its form `key`, not its server `id`: a folder
 * that hasn't been saved yet has no id, and `CollectionMeta` still has to be
 * able to count it.
 *
 * An id the picker can't resolve becomes an unresolved source — the same shape
 * Home uses for a catalog that's since been deleted. In this form that state
 * is also a validation error, so the form names it on the folder; this panel
 * only has to avoid claiming content that isn't there.
 */
function previewFromForm(
  state: CollectionFormState,
  optionByID: ReadonlyMap<string, RefOption>,
): PreviewCollection {
  const { mode, assumed } = normalizeViewMode(state.viewMode)

  const folders: PreviewFolder[] = state.folders.map((folder) => {
    const tile = normalizeTileShape(folder.tileShape)

    const sources: PreviewSource[] = folder.catalogIDs.map((id) => {
      const option = optionByID.get(id)
      return {
        id,
        name: option?.name ?? null,
        type: option?.catalog.type ?? null,
        params: option?.catalog.params ?? '',
      }
    })

    return {
      id: folder.key,
      title: folder.title,
      hideTitle: folder.hideTitle,
      tileShape: tile.shape,
      tileShapeAssumed: tile.assumed,
      coverEmoji: folder.coverEmoji,
      coverImageUrl: folder.coverImageURL,
      sources,
      unresolved: sources.filter((s) => s.name === null).length,
    }
  })

  return {
    // Never rendered — the row's identity is the form, not a stored row, and a
    // collection being created has no id at all.
    id: '',
    title: state.title,
    pinned: state.pinToTop,
    viewMode: mode,
    viewModeAssumed: assumed,
    showAllTab: state.showAllTab,
    hasBackdrop: state.backdropImageURL.trim() !== '',
    folders,
    // The form is the description, so there is always something to draw.
    missing: false,
  }
}
