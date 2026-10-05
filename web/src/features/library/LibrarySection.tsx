import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { Plus, Search } from 'lucide-react'
import { tmdbKind } from '@/api'
import { Icon } from '@/components/Icon'
import { MoreMenu, MoreMenuItem } from '@/components/MoreMenu'
import { railStickers } from '@/features/sharing/sharingState'
import { ListError, ListState } from '@/components/ListState'
import { useHomeSelection } from '@/features/home/useHomeSelection'
import { describeCollection } from './collection'
import { LibraryItem } from './LibraryItem'
import { catalogListing } from './recipe'
import type { Library, LibraryCatalog, LibraryCollection } from './useLibrary'

/**
 * The Library rail: every catalog and collection you own, with its own scroll
 * region and its own name/genre filter.
 *
 * Each row does two independent things: the **toggle** puts it on your home
 * screen or takes it off, and the **row itself** opens it in the pane. Adding
 * something to home and editing it are different intentions, so they never
 * share a target.
 */
export function LibrarySection({
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
  onImport,
  onExport,
  notice,
}: {
  library: Library
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
  onImport: () => void
  onExport: () => void
  /** A short outcome line under the search row, such as the result of an import. */
  notice?: ReactNode
}) {
  const [search, setSearch] = useState('')
  const home = useHomeSelection()

  const query = search.trim().toLowerCase()

  // One pass over each catalog's recipe gives both the summary line a row
  // shows and the text the filter matches, genre names included.
  const catalogRows = useMemo(
    () =>
      library.catalogs.map((catalog) => {
        const { line, searchText } = catalogListing(
          catalog,
          library.genres[tmdbKind(catalog.type)],
        )
        return { catalog, summary: line, searchText }
      }),
    [library.catalogs, library.genres],
  )

  const collectionRows = useMemo(
    () =>
      library.collections.map((collection) => ({
        collection,
        summary: describeCollection(collection),
        searchText: collectionSearchText(collection),
      })),
    [library.collections],
  )

  const catalogs = useMemo(
    () => catalogRows.filter((row) => !query || row.searchText.includes(query)),
    [catalogRows, query],
  )

  const collections = useMemo(
    () => collectionRows.filter((row) => !query || row.searchText.includes(query)),
    [collectionRows, query],
  )

  return (
    <section className="flex flex-col gap-2.5 p-4 lg:min-h-0 lg:flex-1 lg:overflow-y-auto">
      <div className="flex items-center gap-1">
        <div className="relative flex-1">
          <Icon
            icon={Search}
            size={16}
            className="text-dimmer pointer-events-none absolute top-1/2 left-3.5 -translate-y-1/2"
          />
          <input
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Filter by name or genre"
            aria-label="Filter your library"
            className="field h-10 w-full pl-10 text-[14px] pointer-coarse:text-[16px]"
          />
        </div>
        <MoreMenu label="your library">
          <MoreMenuItem onSelect={onImport}>Import JSON</MoreMenuItem>
          <MoreMenuItem onSelect={onExport}>Export JSON</MoreMenuItem>
        </MoreMenu>
      </div>
      {notice}

      {/* One error and one Retry for the whole library: both lists come from
          the same kind of request, and a single refetch reloads whichever
          failed. A group whose own list failed isn't drawn at all; the one
          that loaded keeps its rows. */}
      {library.error && (
        <ListError label="Couldn't load your library." error={library.error} onRetry={library.refetch} />
      )}

      <LibraryGroup
        label="Catalogs"
        tone="catalog"
        action={
          <button type="button" className="sign-btn" onClick={onNewCatalog} aria-label="New catalog">
            <Icon icon={Plus} size={14} />
            New
          </button>
        }
        count={catalogs.length}
        isLoading={library.isLoading}
        failed={library.failed.catalogs}
        emptyLabel={
          query
            ? 'No catalogs match this filter.'
            : 'No catalogs yet. Build one to put a row on your home screen.'
        }
      >
        {catalogs.map(({ catalog, summary }) => (
          <LibraryItem
            key={catalog.id}
            kind={catalog.type}
            stickers={railStickers(catalog)}
            name={catalog.name}
            summary={summary}
            selected={selectedID === catalog.id}
            onSelect={() => onSelectCatalog(catalog)}
            onHome={home.hasCatalog(catalog.id)}
            onToggle={() =>
              home.hasCatalog(catalog.id)
                ? home.removeCatalog(catalog.id)
                : home.addCatalog(catalog.id)
            }
            onDuplicate={() => onDuplicateCatalog(catalog)}
            onDelete={() => onDeleteCatalog(catalog)}
          />
        ))}
      </LibraryGroup>

      <LibraryGroup
        label="Collections"
        tone="collection"
        action={
          <button type="button" className="sign-btn" onClick={onNewCollection} aria-label="New collection">
            <Icon icon={Plus} size={14} />
            New
          </button>
        }
        count={collections.length}
        isLoading={library.isLoading}
        failed={library.failed.collections}
        emptyLabel={
          query
            ? 'No collections match this filter.'
            : 'No collections yet. Build one to group catalogs together.'
        }
      >
        {collections.map(({ collection, summary }) => (
          <LibraryItem
            key={collection.id}
            kind="collection"
            stickers={railStickers(collection)}
            name={collection.title}
            summary={summary}
            selected={selectedID === collection.id}
            onSelect={() => onSelectCollection(collection)}
            onHome={home.hasCollection(collection.id)}
            onToggle={() =>
              home.hasCollection(collection.id)
                ? home.removeCollection(collection.id)
                : home.addCollection(collection.id)
            }
            onDuplicate={() => onDuplicateCollection(collection)}
            onDelete={() => onDeleteCollection(collection)}
          />
        ))}
      </LibraryGroup>
    </section>
  )
}

function LibraryGroup({
  label,
  tone,
  action,
  count,
  isLoading,
  failed,
  emptyLabel,
  children,
}: {
  label: string
  /** The region's colour: its sign. */
  tone: 'catalog' | 'collection'
  action: ReactNode
  count: number
  isLoading: boolean
  /** This list has no rows because its request failed. The group is left out
   *  entirely — the rail's own error above carries the message and the Retry. */
  failed: boolean
  emptyLabel: string
  children: ReactNode
}) {
  if (failed) return null

  return (
    <div className="flex flex-col gap-1">
      {/* The sign spans the rail edge to edge, out through the section's own
          padding; the rows sit 8px in from it. Below `lg` it pins under the
          app header while its own group is scrolling past, as the pane's sign
          does, and the next group's sign pushes it out. */}
      <div className={`sign tone-${tone} sticky top-[var(--app-h)] z-20 -mx-4 mt-1.5 mb-1 lg:static`}>
        <h3 className="type-sign m-0 flex flex-1 items-center gap-2.5">
          {label}
          {!isLoading && <span className="sign-count">{count}</span>}
        </h3>
        {action}
      </div>
      <ListState
        isLoading={isLoading}
        isEmpty={count === 0}
        loadingLabel={`Loading ${label.toLowerCase()}…`}
        emptyLabel={emptyLabel}
      >
        <div className="-mx-2 flex flex-col gap-0.5">{children}</div>
      </ListState>
    </div>
  )
}

/** Folder names carry more signal than a bare count, so search matches them
 *  and the summary shows them until it runs out of room. */
function collectionSearchText(collection: LibraryCollection): string {
  return `${collection.title} ${collection.folders.map((f) => f.title).join(' ')}`.toLowerCase()
}
