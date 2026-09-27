import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { useQuery } from '@tanstack/react-query'
import { fetchCatalogGenreOptions, queryKeys, type Catalog } from '@/api'
import { Grip } from '@/components/dnd'
import { Select } from '@/components/fields'
import { MoreMenu, MoreMenuItem, MoreMenuSeparator } from '@/components/MoreMenu'
import { useHomeSelection } from '@/features/home/useHomeSelection'
import { pluralCount } from '@/lib/plural'
import type { FolderRefState } from './collectionForm'
import { refDragID } from './folderDnd'
import type { RefOption } from './refs'

/**
 * One catalog reference. Order is the value here — index becomes
 * `folder_catalogs.sort_order` — so the position is stated as well as draggable.
 *
 * An option this profile can't resolve is the *same* condition as a save that
 * would 400: `optionByID` merges the library with every scoped catalog this
 * editor already knows about (`CollectionEditor`'s `mergedOptionByID`), which
 * is exactly the closed-graph folder-ref rule `validateFolderRefs` checks
 * server-side.
 *
 * **Quiet Edit is scoped-catalog only.** It opens the referenced catalog one
 * level down, in a modal over this editor — `CollectionEditor`'s own nested
 * `CatalogEditor`, not the main pane — and a scoped catalog is only ever
 * used here, so editing it in place is unambiguous. A *listed* one is a live
 * pointer the same as it always was — editing it here would silently reach
 * every other folder and the library too — so this row has no inline edit
 * for it at all: it states how many places it's used and offers "Copy into
 * this collection" for whoever wants an independent, editable copy instead.
 * Editing a listed catalog directly is the library rail's job.
 */
export function RefRow({
  folderKey,
  refState,
  refKeys,
  siblingGenres,
  position,
  total,
  option,
  usedInFolders,
  onGenreChange,
  onAddGenre,
  onRemove,
  onMove,
  onEdit,
  onCopyIntoCollection,
}: {
  folderKey: string
  refState: FolderRefState
  /** Carried in the drag payload so `onDragEnd` can reorder this folder's list
   *  without reaching back into the form state. */
  refKeys: string[]
  /** The genres this folder's other refs to the same catalog already use —
   *  the pairs this ref may not take, and "Add another genre" may not add. */
  siblingGenres: string[]
  position: number
  total: number
  option: RefOption | undefined
  usedInFolders: (catalogID: string) => number
  onGenreChange: (genre: string) => void
  onAddGenre: (genre: string) => void
  onRemove: () => void
  onMove: (direction: -1 | 1) => void
  onEdit: () => void
  /** Offered on a listed catalog's row only; see `RefMenu`. */
  onCopyIntoCollection: () => void
}) {
  const sortable = useSortable({
    id: refDragID(folderKey, refState.key),
    data: { container: folderKey, ids: refKeys },
  })
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = sortable

  const genreOptions = useGenreOptions(option?.catalog)
  const taken = new Set([refState.genre, ...siblingGenres])
  const nextGenre = genreOptions.data?.find((g) => !taken.has(g.name))?.name

  const isScoped = option ? option.catalog.collection_id !== null : false
  // The home screen is counted here, not in `usedInFolders`, so a change to
  // the selection re-renders the rows that state it and not the workspace.
  const home = useHomeSelection()
  const places =
    option && !isScoped
      ? usedInFolders(refState.catalogID) + (home.hasCatalog(refState.catalogID) ? 1 : 0)
      : 0

  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`run-row ${isDragging ? 'bg-raised relative z-10 opacity-40' : ''}`}
    >
      <div className="run-ctl">
        <Grip label={`Reorder ${option?.name ?? 'unavailable catalog'}`} sortable={{ attributes, listeners }} />
        <span className="type-data text-dimmer w-8 shrink-0 text-right text-[12.5px] tabular-nums">
          {position + 1}
        </span>
      </div>

      <div className="flex min-w-0 items-center gap-3">
        {!option && (
          <span aria-hidden="true" className="text-danger bg-current w-[3px] shrink-0 self-stretch rounded-[1px]" />
        )}

        {option ? (
          <span className="flex min-w-0 flex-1 flex-col gap-0.5">
            <span className="truncate text-[13px] font-medium">{option.name}</span>
            <span className="type-data text-dimmer text-[12.5px] leading-[1.45]">
              {option.recipe} · {option.catalog.type}
              {!isScoped && ` · used in ${pluralCount(places, 'place')}`}
            </span>
            <RefGenrePicker
              query={genreOptions}
              name={option.name}
              genre={refState.genre}
              siblingGenres={siblingGenres}
              onChange={onGenreChange}
            />
          </span>
        ) : (
          <span className="flex min-w-0 flex-1 flex-col gap-0.5">
            <span className="text-danger truncate text-[13px]">Unavailable catalog</span>
            <span className="type-data text-dimmer truncate text-[12.5px]">Deleted</span>
          </span>
        )}

        <div className="flex shrink-0 items-center gap-1">
          {isScoped ? (
            <button type="button" onClick={onEdit} className="btn-ghost px-2 text-[12px]">
              Edit
            </button>
          ) : !option ? (
            <button type="button" onClick={onRemove} className="btn-ghost px-2 text-[12px]">
              Remove
            </button>
          ) : null}
          <RefMenu
            label={option?.name ?? 'this catalog'}
            first={position === 0}
            last={position === total - 1}
            onMove={onMove}
            onRemove={onRemove}
            nextGenre={nextGenre}
            onAddGenre={option ? onAddGenre : undefined}
            onCopyIntoCollection={option && !isScoped ? onCopyIntoCollection : undefined}
          />
        </div>
      </div>
    </li>
  )
}

/**
 * The genres a pick can narrow `catalog`'s recipe by
 * (`POST /api/catalogs/genre-options`) — the list the manifest advertises and
 * the addon path resolves against. Idle for an unresolvable ref, which has no
 * recipe to ask about.
 */
function useGenreOptions(catalog: Pick<Catalog, 'type' | 'params'> | undefined) {
  return useQuery({
    queryKey: queryKeys.catalogGenreOptions(catalog?.type ?? 'movie', catalog?.params ?? ''),
    queryFn: () => fetchCatalogGenreOptions({ type: catalog!.type, params: catalog!.params }),
    enabled: catalog !== undefined,
    staleTime: 5 * 60_000,
    retry: false,
  })
}

/**
 * Narrows one folder reference to a genre — pushed as its source's `genre`,
 * which Nuvio sends back as the catalog's genre extra when it loads the row.
 *
 * Offers the recipe's own genre options, so every choice here actually narrows
 * the row, minus the genres this folder's other refs to the same catalog
 * already use: the same catalog under the same genre twice is a repeat the
 * primary key forbids. A stored genre that's no longer among the options — the
 * recipe has since been edited to require or exclude it — is kept and flagged
 * rather than cleared: the addon path serves that row unfiltered, and the user
 * decides what to do about it.
 */
function RefGenrePicker({
  query,
  name,
  genre,
  siblingGenres,
  onChange,
}: {
  query: ReturnType<typeof useGenreOptions>
  name: string
  genre: string
  siblingGenres: string[]
  onChange: (genre: string) => void
}) {
  const offered = query.data ?? []
  const stale = genre !== '' && query.isSuccess && !offered.some((g) => g.name === genre)
  const free = (value: string) => value === genre || !siblingGenres.includes(value)
  const options = [
    ...(free('') ? [{ value: '', label: 'All genres' }] : []),
    // "Only …" rather than the bare name, so the closed control says what it
    // does without a label of its own.
    ...offered.filter((g) => free(g.name)).map((g) => ({ value: g.name, label: `Only ${g.name}` })),
    // Until the list lands (or if it can't), the stored value still needs an
    // option to show as selected.
    ...(genre !== '' && !offered.some((g) => g.name === genre)
      ? [{ value: genre, label: stale ? `Only ${genre} (no longer applies)` : `Only ${genre}` }]
      : []),
  ]

  return (
    <span className="mt-1.5 flex flex-col gap-1">
      <Select
        value={genre}
        onChange={onChange}
        options={options}
        width="var(--w-pick)"
        ariaLabel={`Genre shown from ${name}`}
      />
      {stale && (
        <span className="type-data text-danger text-[12.5px] leading-[1.45]">
          This catalog's filters no longer allow {genre}, so Nuvio shows it unfiltered.
        </span>
      )}
      {query.isError && (
        <span className="type-data text-dimmer text-[12.5px] leading-[1.45]">
          Couldn't load this catalog's genres.
        </span>
      )}
    </span>
  )
}

/**
 * Everything on a catalog row besides Edit: Edit is the row's one inline
 * action, and dragging the grip (pointer, touch or keyboard) is the main way
 * to reorder, so "Add another genre", Move up/down, Remove and "Copy into this
 * collection" wait behind "⋯" and the name keeps the row's width at phone size.
 */
function RefMenu({
  label,
  first,
  last,
  onMove,
  onRemove,
  nextGenre,
  onAddGenre,
  onCopyIntoCollection,
}: {
  label: string
  first: boolean
  last: boolean
  onMove: (direction: -1 | 1) => void
  onRemove: () => void
  /** The next genre this ref's catalog can be narrowed by. Absent before the
   *  genre options land, and once every one of them is taken by a ref to this
   *  catalog in this folder. */
  nextGenre?: string
  /** Absent for an unavailable catalog, which has no genres to offer. */
  onAddGenre?: (genre: string) => void
  /** Absent for a scoped or unavailable catalog — nothing else can reference
   *  it, so there's nothing to copy it away from. */
  onCopyIntoCollection?: () => void
}) {
  return (
    <MoreMenu label={label}>
      {onAddGenre && (
        <MoreMenuItem
          disabled={nextGenre === undefined}
          onSelect={() => {
            if (nextGenre !== undefined) onAddGenre(nextGenre)
          }}
        >
          Add another genre
        </MoreMenuItem>
      )}
      {onCopyIntoCollection && (
        <MoreMenuItem onSelect={onCopyIntoCollection}>Copy into this collection</MoreMenuItem>
      )}
      {(onAddGenre || onCopyIntoCollection) && <MoreMenuSeparator />}
      <MoreMenuItem disabled={first} onSelect={() => onMove(-1)}>
        Move up
      </MoreMenuItem>
      <MoreMenuItem disabled={last} onSelect={() => onMove(1)}>
        Move down
      </MoreMenuItem>
      <MoreMenuSeparator />
      <MoreMenuItem onSelect={onRemove}>Remove from folder</MoreMenuItem>
    </MoreMenu>
  )
}
