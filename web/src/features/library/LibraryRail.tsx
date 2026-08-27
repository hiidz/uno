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
 * Purely presentational: what a selection *does* is the workspace's business,
 * because the same unsaved-changes guard covers every other way out of an
 * editor too.
 */
export function LibraryRail({
  library,
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
    <aside className="bg-sidebar border-line flex flex-col lg:min-h-0 lg:border-r">
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
