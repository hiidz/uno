import { useEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent } from 'react'
import { pluralCount } from '@/lib/plural'
import { FOLDER_LAYOUT_LABEL, TVCollectionRow, TVFolderPage } from '@/features/home/tv'
import { CollectionMeta } from '@/features/preview/CollectionMeta'
import {
  normalizeTileShape,
  normalizeViewMode,
  type PreviewCollection,
  type PreviewFolder,
  type PreviewSource,
} from '@/features/preview/model'
import { useRecipesTiles, type TileRecipe } from '@/features/preview/useRecipesTiles'
import type { CollectionFormState } from './collectionForm'
import type { RefOption } from './refs'

/**
 * "On your TV" — DESIGN.md's docked panel beside the collection form: the
 * collection's row from the live draft, drawn with the Home preview's own TV
 * components, and the folder pages it opens.
 *
 * **A crop of a real-scale TV** (`.tv-crop`): tiles are the Home preview's
 * size however narrow the column, rows run off the frame's edge and scroll,
 * and a folder page's grid reflows to the frame.
 *
 * **Folders open, exactly as on Home.** A folder tile opens `TVFolderPage` —
 * tabs or rows per the draft's `view_mode`, real titles per catalog. Tiles
 * are fetched from each catalog's recipe (`useRecipesTiles`), not its id, so
 * an unsaved draft and a Community collection both preview, and only the
 * open folder's catalogs are fetched. The in-screen back arrow and Escape
 * return to the row; Escape is taken here, while focus is inside the screen,
 * so it never reaches the editor's own Escape-to-close. There is no history
 * entry — the browser's Back belongs to the editor, not to this panel.
 */
export function CollectionPreview({
  state,
  optionByID,
}: {
  state: CollectionFormState
  optionByID: ReadonlyMap<string, RefOption>
}) {
  const collection = useMemo(() => previewFromForm(state, optionByID), [state, optionByID])
  const title = collection.title.trim() || 'Untitled collection'
  const empty = collection.folders.length === 0

  const [openKey, setOpenKey] = useState<string | null>(null)
  // A folder removed from the form while its page is open falls back to the row.
  const folder = openKey === null ? null : (collection.folders.find((f) => f.id === openKey) ?? null)

  const recipes = useMemo<TileRecipe[]>(
    () =>
      (folder?.sources ?? []).flatMap((source) =>
        source.type === null
          ? []
          : [{ id: source.key, type: source.type, params: source.params, genre: source.genre }],
      ),
    [folder],
  )
  const tiles = useRecipesTiles(recipes)

  const screenRef = useRef<HTMLDivElement>(null)
  // The tile a folder page was opened from, so leaving it hands focus back
  // there instead of dropping it on the page when the page unmounts.
  const openedFrom = useRef<number | null>(null)
  const isOpen = folder !== null

  useEffect(() => {
    const screen = screenRef.current
    if (!screen) return
    screen.scrollTop = 0
    if (isOpen) {
      screen.querySelector<HTMLElement>('.fp-back')?.focus({ preventScroll: true })
    } else if (openedFrom.current !== null) {
      screen.querySelectorAll<HTMLElement>('.tv-folder')[openedFrom.current]?.focus({ preventScroll: true })
      openedFrom.current = null
    }
  }, [isOpen])

  function onScreenKeyDown(e: ReactKeyboardEvent<HTMLDivElement>) {
    if (e.key !== 'Escape' || !isOpen) return
    e.stopPropagation()
    setOpenKey(null)
  }

  const shown = { ...collection, title }

  return (
    <div className="ed-pv">
      <div className="ed-pv-head">
        <span className="type-eyebrow">On your TV</span>
      </div>

      <div className="tv-bezel tv-crop">
        <div
          ref={screenRef}
          className="tv-screen"
          tabIndex={0}
          role="region"
          onKeyDown={onScreenKeyDown}
          aria-label={
            folder
              ? `${folder.title || 'Untitled folder'}, a folder in ${title}, as your TV shows it`
              : empty
                ? `${title}, an empty row on your TV`
                : `${title}, a row of ${pluralCount(collection.folders.length, 'folder')} on your TV`
          }
        >
          <div className="tv-crop-stage">
            <div className="tv-crop-view">
              {folder ? (
                <TVFolderPage
                  collection={shown}
                  folder={folder}
                  tiles={tiles}
                  onBack={() => setOpenKey(null)}
                  backLabel={`Back to the ${title} row`}
                />
              ) : (
                <TVCollectionRow
                  collection={shown}
                  onOpenFolder={({ folderId }) => {
                    openedFrom.current = collection.folders.findIndex((f) => f.id === folderId)
                    setOpenKey(folderId)
                  }}
                />
              )}
            </div>
          </div>
        </div>
      </div>

      <p>
        {folder
          ? `${FOLDER_LAYOUT_LABEL[collection.viewMode]}. The arrow beside the folder name goes back to the row, and so does Esc.`
          : empty
            ? 'No folders yet, so this row is empty.'
            : 'Open a folder to see its catalogs the way your TV shows them.'}
      </p>

      <CollectionMeta collection={collection} />
    </div>
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

    const sources: PreviewSource[] = folder.refs.map((ref) => {
      const option = optionByID.get(ref.catalogID)
      return {
        // The ref's own key, not the catalog/genre pair: the form can briefly
        // hold a repeated pair, which is a validation error rather than a
        // state the preview may collide on.
        key: ref.key,
        id: ref.catalogID,
        name: option?.name ?? null,
        type: option?.catalog.type ?? null,
        params: option?.catalog.params ?? '',
        genre: ref.genre,
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
