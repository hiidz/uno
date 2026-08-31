import { useMemo, useState } from 'react'
import { tmdbKind } from '@/api'
import { ListState } from '@/components/ListState'
import { useHomeSelection } from '@/features/home/useHomeSelection'
import { LibraryItem } from './LibraryItem'
import { catalogSearchText } from './recipe'
import type { LibraryCatalog, LibraryCollection, useLibrary } from './useLibrary'

/**
 * One half of the Library rail — either "Mine" or "Community" — with its own
 * scroll region and its own name/genre filter, so narrowing one half never
 * touches what the other shows.
 *
 * Every catalog and collection owned by this half, as the source its two
 * groups are filled from. Each row does two independent things: the
 * **toggle** puts it on your home screen or takes it off, and the **row
 * itself** opens it in the pane. Adding something to home and editing it are
 * different intentions, so they never share a target.
 */
export function LibrarySection({
  owned,
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
  owned: boolean
  library: ReturnType<typeof useLibrary>
  /** The row whose editor is open in the pane, or `null` for none. */
  selectedID: string | null
  /** Only the "Mine" section can create rows — a community row is adopted by
   *  opening it, never created directly. */
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
  const label = owned ? 'Mine' : 'Community'

  // `catalogSearchText` still parses each catalog's params JSON so genre names
  // remain searchable, even though the rail no longer displays the recipe.
  const catalogRows = useMemo(
    () =>
      library.catalogs
        .filter((catalog) => catalog.owned === owned)
        .map((catalog) => {
          const lookup = library.genres[tmdbKind(catalog.type)]
          return {
            catalog,
            summary: catalog.type === 'movie' ? 'Movies' : 'Series',
            searchText: catalogSearchText(catalog, lookup),
          }
        }),
    [library.catalogs, library.genres, owned],
  )

  const collectionRows = useMemo(
    () =>
      library.collections
        .filter((collection) => collection.owned === owned)
        .map((collection) => ({
          collection,
          summary: describeCollection(collection),
          searchText: collectionSearchText(collection),
        })),
    [library.collections, owned],
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
        <span className="type-eyebrow">{label}</span>
      </div>
      <input
        type="search"
        value={search}
        onChange={(event) => setSearch(event.target.value)}
        placeholder="Filter by name or genre…"
        aria-label={`Filter ${label.toLowerCase()}`}
        className="field type-data w-full text-[12px] pointer-coarse:text-[16px]"
      />

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
        error={library.error}
        onRetry={library.refetch}
        errorLabel="Couldn't load catalogs."
        emptyLabel={
          query
            ? 'No catalogs match this filter.'
            : owned
              ? 'No catalogs yet. Build one to put a row on your home screen.'
              : 'No community catalogs yet.'
        }
      >
        {catalogs.map(({ catalog, summary }) => (
          <LibraryItem
            key={catalog.id}
            kind={catalog.type}
            owned={catalog.owned}
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
            onDuplicate={catalog.owned ? () => onDuplicateCatalog(catalog) : undefined}
            onDelete={catalog.owned ? () => onDeleteCatalog(catalog) : undefined}
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
        error={library.error}
        onRetry={library.refetch}
        errorLabel="Couldn't load collections."
        emptyLabel={
          query
            ? 'No collections match this filter.'
            : owned
              ? 'No collections yet. Build one to group catalogs together.'
              : 'No community collections yet.'
        }
      >
        {collections.map(({ collection, summary }) => (
          <LibraryItem
            key={collection.id}
            kind="collection"
            owned={collection.owned}
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
            onDuplicate={collection.owned ? () => onDuplicateCollection(collection) : undefined}
            onDelete={collection.owned ? () => onDeleteCollection(collection) : undefined}
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
  error,
  onRetry,
  errorLabel,
  emptyLabel,
  children,
}: {
  label: string
  action?: React.ReactNode
  count: number
  isLoading: boolean
  error: Error | null
  onRetry: () => void
  errorLabel: string
  emptyLabel: string
  children: React.ReactNode
}) {
  return (
    <div className="flex flex-col gap-1">
      <div className="border-line mb-1 flex items-center gap-2 border-b pb-2">
        <span className="type-eyebrow flex-1">{label}</span>
        {!isLoading && !error && (
          <span className="type-data text-dimmer text-[10px]">{count}</span>
        )}
        {action}
      </div>
      <ListState
        isLoading={isLoading}
        error={error}
        isEmpty={count === 0}
        loadingLabel={`Loading ${label.toLowerCase()}…`}
        errorLabel={errorLabel}
        emptyLabel={emptyLabel}
        onRetry={onRetry}
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

function describeCollection(collection: LibraryCollection): string {
  const n = collection.folders.length
  const count = `${n} ${n === 1 ? 'folder' : 'folders'}`
  if (n === 0) return count
  return `${count} · ${collection.folders.map((f) => f.title).join(', ')}`
}
