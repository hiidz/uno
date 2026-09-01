import { useMemo, useState } from 'react'
import { TypeBar } from '@/components/TypeBar'
import { CollectionMeta } from '@/features/preview/CollectionMeta'
import { FolderPage } from '@/features/preview/FolderPage'
import type { PreviewChrome } from '@/features/preview/FolderPage'
import {
  folderRecipes,
  normalizeTileShape,
  normalizeViewMode,
  type PreviewCollection,
  type PreviewFolder,
  type PreviewSource,
} from '@/features/preview/model'
import { FolderTile, Note } from '@/features/preview/tiles'
import { useRecipesTiles } from '@/features/preview/useRecipesTiles'
import type { CollectionFormState } from './collectionForm'
import type { RefOption } from './refs'

/**
 * What the collection being edited will look like.
 *
 * **Layout, not content.** A collection has no recipe of its own — it's folders
 * of catalog references — so there is nothing to run against TMDB at this
 * level. What it does have is a shape: folders as tiles at their own
 * `tile_shape`, and a `view_mode` deciding what a folder opens into. That shape
 * comes entirely from form state, so the row below costs no request and is
 * drawn without asking.
 *
 * Content appears one level down, where a catalog finally has a recipe behind
 * it: opening a folder tile fetches that folder's catalogs. Opening a folder is
 * itself the request, so nothing is fetched until someone asks for it.
 *
 * Renders the same components as the Home pane's Preview view, from the same
 * model, so the two can't disagree about what a layout will do.
 */
export function CollectionPreview({
  state,
  optionByID,
}: {
  state: CollectionFormState
  optionByID: ReadonlyMap<string, RefOption>
}) {
  const collection = useMemo(
    () => previewFromForm(state, optionByID),
    [state, optionByID],
  )

  // Which folder page is open, held as the folder's form `key`. A folder
  // removed from the tree while its page is open no longer resolves, and the
  // view falls back to the row rather than showing a different folder now at
  // the same position.
  const [openKey, setOpenKey] = useState<string | null>(null)
  const openFolder = openKey ? collection.folders.find((f) => f.id === openKey) : undefined

  return (
    <section className="border-line mt-6 flex flex-col gap-3 border-t pt-5">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <span className="type-eyebrow flex-1">Preview</span>
        <span className="type-data text-dimmer text-[10.5px]">
          shows layout only — open a folder to see content
        </span>
      </div>

      <div className="border-line bg-ground flex flex-col gap-4 rounded-[2px] border p-5">
        {openFolder ? (
          <OpenFolder
            collection={collection}
            folder={openFolder}
            optionByID={optionByID}
            onBack={() => setOpenKey(null)}
          />
        ) : (
          <CollectionRow collection={collection} onOpenFolder={setOpenKey} />
        )}
      </div>
    </section>
  )
}

/** The collection as one row on home: its tiles are its folders, never its
 *  content. Ragged by design — `tile_shape` is per folder, so folders in one
 *  collection can disagree and the real row is uneven too. */
function CollectionRow({
  collection,
  onOpenFolder,
}: {
  collection: PreviewCollection
  onOpenFolder: (folderKey: string) => void
}) {
  return (
    <>
      <div className="flex flex-wrap items-center gap-2.5">
        {/* Always solid: you can only edit a collection that's yours. */}
        <TypeBar kind="collection" owned className="h-[13px] self-auto" />
        <h3 className="m-0 text-[13px] font-medium">
          {collection.title || 'Untitled collection'}
        </h3>
        {collection.pinned && <Note>pinned to the top of home</Note>}
      </div>

      {collection.folders.length === 0 ? (
        <p className="type-data text-dimmer m-0 text-[11px]">
          No folders yet, so this collection's row is empty. Add one above.
        </p>
      ) : (
        <div className="flex items-end gap-3 overflow-x-auto overscroll-x-contain">
          {collection.folders.map((folder) => (
            <FolderTile
              key={folder.id}
              folder={folder}
              onOpen={() => onOpenFolder(folder.id)}
            />
          ))}
        </div>
      )}

      <CollectionMeta collection={collection} />
    </>
  )
}

/**
 * One folder's page, with its catalogs' content fetched on open.
 *
 * A separate component so the fetch is mounted with the page: closing the page
 * unmounts it, and nothing is requested for a folder nobody opened.
 */
function OpenFolder({
  collection,
  folder,
  optionByID,
  onBack,
}: {
  collection: PreviewCollection
  folder: PreviewFolder
  optionByID: ReadonlyMap<string, RefOption>
  onBack: () => void
}) {
  const recipes = useMemo(() => folderRecipes(folder), [folder])
  const tiles = useRecipesTiles(recipes)

  const chrome: PreviewChrome = useMemo(
    () => ({
      // The library is `owned ∪ is_public`, and a folder can only reference
      // what's in it, so ownership is whatever the picker already knows. There
      // is no `note` — "no longer in the library" is a Home-selection question,
      // and here an unavailable reference is a validation error the form
      // reports on the folder itself.
      isOwned: (id) => optionByID.get(id)?.catalog.owned ?? false,
    }),
    [optionByID],
  )

  return (
    <FolderPage
      collection={collection}
      folder={folder}
      tiles={tiles}
      chrome={chrome}
      onBack={onBack}
      backLabel="← Back"
    />
  )
}

/**
 * The form's own state as a previewable collection.
 *
 * A folder's identity here is its form `key`, not its server `id`: a folder
 * that hasn't been saved yet has no id, and the preview still has to be able to
 * open it.
 *
 * An id the picker can't resolve becomes an unresolved source — the same shape
 * Home uses for a catalog whose owner made it private. In this form that state
 * is also a validation error, so the form names it on the folder; the preview
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
