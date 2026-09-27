import { useEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent } from 'react'
import { pluralCount } from '@/lib/plural'
import { FOLDER_LAYOUT_LABEL, TVCollectionRow, TVFolderPage } from '@/features/home/tv'
import { CollectionMeta } from '@/features/preview/CollectionMeta'
import { folderRecipes, type PreviewCollection } from '@/features/preview/model'
import { useRecipesTiles } from '@/features/preview/useRecipesTiles'

/**
 * "On your TV" — DESIGN.md's docked panel beside the collection form: the
 * collection's row, drawn with the Home preview's own TV components, and the
 * folder pages it opens. The editor passes its live draft (`previewFromForm`),
 * Community a saved collection (`toPreviewCollection`); both memoise it, since
 * the open folder page is found by folder id and its tiles by the folder's
 * sources.
 *
 * **Rows in the raised panel** (`.pv-rows`): tiles are the Home preview's
 * size however narrow the column, rows run off the panel's edge and scroll,
 * and a folder page's grid reflows to the panel. Docked beside the editor's
 * form, the rows scroll inside the panel; anywhere else they flow with the
 * page.
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
export function CollectionPreview({ collection }: { collection: PreviewCollection }) {
  const title = collection.title.trim() || 'Untitled collection'
  const empty = collection.folders.length === 0

  const [openKey, setOpenKey] = useState<string | null>(null)
  // A folder removed from the form while its page is open falls back to the row.
  const folder = openKey === null ? null : (collection.folders.find((f) => f.id === openKey) ?? null)

  const recipes = useMemo(() => (folder ? folderRecipes(folder) : []), [folder])
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
        <span className="type-label">Preview</span>
      </div>

      <div
        ref={screenRef}
        className="pv-rows"
        tabIndex={0}
        role="region"
        onKeyDown={onScreenKeyDown}
        aria-label={
          folder
            ? `${folder.title || 'Untitled folder'}, a folder in ${title}`
            : empty
              ? `${title}, an empty row`
              : `${title}, a row of ${pluralCount(collection.folders.length, 'folder')}`
        }
      >
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

      {folder ? <p>{FOLDER_LAYOUT_LABEL[collection.viewMode]}</p> : empty && <p>No folders yet.</p>}

      <CollectionMeta collection={collection} />
    </div>
  )
}
