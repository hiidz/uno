import { useMemo, useState } from 'react'
import { tmdbKind } from '@/api'
import { ListState } from '@/components/ListState'
import { useHomeSelection } from '@/features/home/useHomeSelection'
import { describeCollection } from './collection'
import { LibraryItem } from './LibraryItem'
import { catalogSearchText } from './recipe'
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
}: {
  library: Library
  /** The row whose editor is open in the pane, or `null` for none. */
  selectedID: string | null
  onNewCatalog?: () => void
  onNewCollection?: () => void
  onSelectCatalog: (catalog: LibraryCatalog) => void
  onSelectCollection: (collection: LibraryCollection) => void
  onDuplicateCatalog: (catalog: LibraryCatalog) => void
  onDuplicateCollection: (collection: LibraryCollection) => void
  onDeleteCatalog: (catalog: LibraryCatalog) => void
  onDeleteCollection: (collection: LibraryCollection) => void
}) {
  const [search, setSearch] = useState('')
  const home = useHomeSelection()

  const query = search.trim().toLowerCase()

  // `catalogSearchText` still parses each catalog's params JSON so genre names
  // remain searchable, even though the rail no longer displays the recipe.
  const catalogRows = useMemo(
    () =>
      library.catalogs.map((catalog) => {
        const lookup = library.genres[tmdbKind(catalog.type)]
        return {
          catalog,
          summary: catalog.type === 'movie' ? 'Movies' : 'Series',
          searchText: catalogSearchText(catalog, lookup),
        }
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
    <section className="border-line flex flex-col gap-2.5 border-b p-4 last:border-b-0 lg:min-h-0 lg:flex-1 lg:overflow-y-auto">
      <div className="flex items-center justify-between gap-2">
        <span className="type-eyebrow">Mine</span>
      </div>
      <input
        type="search"
        value={search}
        onChange={(event) => setSearch(event.target.value)}
        placeholder="Filter by name or genre…"
        aria-label="Filter your library"
        className="field type-data w-full text-[12px] pointer-coarse:text-[16px]"
      />

      {/* One error and one Retry for the whole library: both lists come from
          the same kind of request, and a single refetch reloads whichever
          failed. A group whose own list failed isn't drawn at all; the one
          that loaded keeps its rows. */}
      <ListState
        isLoading={false}
        error={library.error}
        isEmpty={false}
        errorLabel="Couldn't load your library."
        emptyLabel={null}
        onRetry={library.refetch}
      >
        {null}
      </ListState>

      <LibraryGroup
        label="Catalogs"
        action={
          onNewCatalog && (
            <button type="button" className="btn-ghost" onClick={onNewCatalog}>
              New
            </button>
          )
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
            isPublic={catalog.is_public}
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
        action={
          onNewCollection && (
            <button type="button" className="btn-ghost" onClick={onNewCollection}>
              New
            </button>
          )
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
            isPublic={collection.is_public}
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
  action,
  count,
  isLoading,
  failed,
  emptyLabel,
  children,
}: {
  label: string
  action?: React.ReactNode
  count: number
  isLoading: boolean
  /** This list has no rows because its request failed. The group is left out
   *  entirely — the rail's own error above carries the message and the Retry. */
  failed: boolean
  emptyLabel: string
  children: React.ReactNode
}) {
  if (failed) return null

  return (
    <div className="flex flex-col gap-1">
      <div className="border-line mb-1 flex items-center gap-2 border-b pb-2">
        <span className="type-eyebrow flex-1">{label}</span>
        {!isLoading && (
          <span className="type-data text-dimmer text-[10px]">{count}</span>
        )}
        {action}
      </div>
      <ListState
        isLoading={isLoading}
        error={null}
        isEmpty={count === 0}
        loadingLabel={`Loading ${label.toLowerCase()}…`}
        errorLabel=""
        emptyLabel={emptyLabel}
      >
        {children}
      </ListState>
    </div>
  )
}

/** Folder names carry more signal than a bare count, so search matches them
 *  and the summary shows them until it runs out of room. */
function collectionSearchText(collection: LibraryCollection): string {
  return `${collection.title} ${collection.folders.map((f) => f.title).join(' ')}`.toLowerCase()
}
