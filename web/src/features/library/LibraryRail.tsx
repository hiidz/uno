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
 * Below `lg` the rail is a screen of its own rather than a column, so it
 * carries a link to home — the pane is not visible beside it to be clicked.
 *
 * Purely presentational: what a selection *does* is the workspace's business,
 * because the same unsaved-changes guard covers every other way out of an
 * editor too.
 */
export function LibraryRail({
  library,
  className,
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
  library: ReturnType<typeof useLibrary>
  /** Carries the rail's own visibility, which only the workspace knows: below
   *  `lg` the two regions take turns, and which one is up is state held there.
   *  Whatever this passes, `lg:flex` wins from `lg` up. */
  className?: string
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
      className={`bg-sidebar border-line flex-col lg:flex lg:min-h-0 lg:border-r ${className ?? 'flex'}`}
    >
      {/* Above `lg` home is simply the other half of the screen and needs no
          link. Below it, the pane is off screen while the rail is up, so this
          is the only way back to it — and the rail is where the user is when
          they have just closed an editor. */}
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
          ›
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
