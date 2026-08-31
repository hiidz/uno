import type { RefObject } from 'react'
import { LibrarySection } from './LibrarySection'
import type { LibraryCatalog, LibraryCollection, useLibrary } from './useLibrary'

/**
 * The sidebar: your own catalogs and collections on top, everyone else's
 * below. The two halves used to be one filtered list — a control choosing
 * which owner to look at — but "mine" and "community" are different tasks
 * (build here, browse there), so each now gets its own permanently visible
 * section with its own scroll region and its own name/genre filter. Only
 * "Mine" gets the New buttons; you can't create a community row, only adopt
 * one by opening it.
 *
 * Below `lg` the rail is the top of one long page rather than a column, with
 * the pane stacked underneath it. It keeps its link to home, which stops being
 * a way *across* to the pane and becomes a shortcut *down* to it.
 *
 * Purely presentational: what a selection *does* is the workspace's business,
 * because the same unsaved-changes guard covers every other way out of an
 * editor too.
 */
export function LibraryRail({
  scrollRef,
  library,
  homeSelected,
  onShowHome,
  selectedID,
  onNewCatalog,
  onNewCollection,
  onSelectCatalog,
  onSelectCollection,
  onDuplicateCatalog,
  onDuplicateCollection,
  onDeleteCatalog,
  onDeleteCollection,
}: {
  /** Below `lg` this region is a scroll destination, and the workspace is what
   *  scrolls to it. */
  scrollRef?: RefObject<HTMLElement | null>
  library: ReturnType<typeof useLibrary>
  /** The pane is holding home rather than an editor. */
  homeSelected: boolean
  onShowHome: () => void
  /** The row whose editor is open in the pane, or `null` for none. */
  selectedID: string | null
  onNewCatalog: () => void
  onNewCollection: () => void
  onSelectCatalog: (catalog: LibraryCatalog) => void
  onSelectCollection: (collection: LibraryCollection) => void
  onDuplicateCatalog: (catalog: LibraryCatalog) => void
  onDuplicateCollection: (collection: LibraryCollection) => void
  onDeleteCatalog: (catalog: LibraryCatalog) => void
  onDeleteCollection: (collection: LibraryCollection) => void
}) {
  return (
    <aside
      ref={scrollRef}
      tabIndex={-1}
      data-landing
      aria-label="Library"
      className="bg-sidebar border-line flex scroll-mt-[var(--app-h)] flex-col outline-none lg:min-h-0 lg:border-r"
    >
      {/* Above `lg` home is simply the other half of the screen and needs no
          link. Below it the pane is further down the same page, so this is a
          shortcut to it rather than a way across — hence `↓` and not `›`. It
          still guards, because arriving at home means the open editor is
          replaced by it. */}
      <button
        type="button"
        onClick={onShowHome}
        aria-current={homeSelected ? 'true' : undefined}
        className={`border-line hover:bg-raised flex items-center gap-3 border-b px-4 py-3 text-left transition-colors lg:hidden ${
          homeSelected ? 'bg-raised-hi' : ''
        }`}
      >
        <span className="type-display flex-1 text-[12px]">Your home screen</span>
        <span aria-hidden="true" className="type-data text-dimmer text-[13px] leading-none">
          ↓
        </span>
      </button>

      <LibrarySection
        owned
        library={library}
        selectedID={selectedID}
        onNewCatalog={onNewCatalog}
        onNewCollection={onNewCollection}
        onSelectCatalog={onSelectCatalog}
        onSelectCollection={onSelectCollection}
        onDuplicateCatalog={onDuplicateCatalog}
        onDuplicateCollection={onDuplicateCollection}
        onDeleteCatalog={onDeleteCatalog}
        onDeleteCollection={onDeleteCollection}
      />
      <LibrarySection
        owned={false}
        library={library}
        selectedID={selectedID}
        onSelectCatalog={onSelectCatalog}
        onSelectCollection={onSelectCollection}
        onDuplicateCatalog={onDuplicateCatalog}
        onDuplicateCollection={onDuplicateCollection}
        onDeleteCatalog={onDeleteCatalog}
        onDeleteCollection={onDeleteCollection}
      />
    </aside>
  )
}
