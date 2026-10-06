import { useState, type ReactNode } from 'react'
import { SortableContext, rectSortingStrategy, useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { useQuery } from '@tanstack/react-query'
import { Plus, X } from 'lucide-react'
import { Popover } from 'radix-ui'
import { fetchCatalogGenreOptions, queryKeys, type Catalog } from '@/api'
import { GripIcon, Icon } from '@/components/Icon'
import type { FolderRefState, FolderUnit, RefGroup } from './collectionForm'
import { dragID, genreContainer, type ReorderableData } from './folderDnd'
import { genreChoices, isUnsplit, type GenreChoice } from './folderEdits'

export interface GenreSplitProps {
  folderKey: string
  group: RefGroup
  catalog: Pick<Catalog, 'type' | 'params'>
  /** The catalog's name, for the controls' accessible names. */
  name: string
  unit: FolderUnit
  onAddGenre(genre: string): void
  onRemoveRef(refKey: string): void
  onReorder(refKeys: string[]): void
}

/**
 * A folder catalog split by genre. Each of its refs is one Nuvio tab (or
 * row), and push sends each ref's genre as that source's `genre`, which
 * Nuvio sends back as the catalog's genre extra when it loads it — so one
 * catalog makes several filtered tabs with no second catalog.
 *
 * Unsplit (one ref, no genre filter) it is a single "Split by genre" link.
 * Split, it is a chip per ref, in tab order and draggable, then "Genre".
 * Both open the same dropdown of genres to tick. The dropdown stays at one
 * place in the tree, its open state held here, so the first tick — which
 * turns the link into chips — leaves it open for the next.
 */
export function GenreSplit(props: GenreSplitProps) {
  const [open, setOpen] = useState(false)
  const query = useGenreOptions(props.catalog)
  const choices = genreChoices(offeredNames(query.data, query.isSuccess), props.group.refs)
  const dropdown = { choices, failed: query.isError, open, onOpenChange: setOpen, ...dropdownProps(props) }
  const split = !isUnsplit(props.group.refs)
  const stale = staleGenres(choices)
  return (
    <span className="mt-1.5 flex flex-col gap-1.5">
      {split && (
        <span className="text-dimmer text-[12.5px]">
          <span className="text-dim font-semibold">Split by genre</span> · one {props.unit} per genre
        </span>
      )}
      <span className="flex flex-wrap items-center gap-1.5">
        {split && <GenreChips props={props} stale={stale} />}
        <GenreDropdown dropdown={dropdown}>
          {split ? (
            <button
              type="button"
              aria-label={`Genres of ${props.name}`}
              className="text-catalog border-catalog/60 hover:bg-catalog/10 inline-flex h-7 items-center gap-1 rounded-full border border-dashed px-3 text-[13px] font-semibold"
            >
              <Icon icon={Plus} size={13} />
              Genre
            </button>
          ) : (
            <button type="button" className="text-catalog inline-flex w-max items-center gap-1 text-[12.5px] font-semibold hover:underline">
              <Icon icon={Plus} size={13} />
              Split by genre
            </button>
          )}
        </GenreDropdown>
      </span>
      {stale.map((genre) => (
        <span key={genre} className="type-data text-danger text-[12.5px] leading-[1.45]">
          This catalog's filters no longer allow {genre}, so Nuvio shows that {props.unit} unfiltered.
        </span>
      ))}
    </span>
  )
}

/**
 * The genres a pick can narrow `catalog`'s recipe by
 * (`POST /api/catalogs/genre-options`) — the list the manifest advertises and
 * the addon path resolves against.
 */
function useGenreOptions(catalog: Pick<Catalog, 'type' | 'params'>) {
  return useQuery({
    queryKey: queryKeys.catalogGenreOptions(catalog.type, catalog.params),
    queryFn: () => fetchCatalogGenreOptions({ type: catalog.type, params: catalog.params }),
    staleTime: 5 * 60_000,
    retry: false,
  })
}

function offeredNames(data: { name: string }[] | undefined, loaded: boolean): string[] | undefined {
  if (!loaded || !data) return undefined
  return data.map((genre) => genre.name)
}

interface DropdownProps {
  choices: GenreChoice[]
  failed: boolean
  open: boolean
  onOpenChange(open: boolean): void
  name: string
  unit: FolderUnit
  onAddGenre(genre: string): void
  onRemoveRef(refKey: string): void
}

function dropdownProps(props: GenreSplitProps): Omit<DropdownProps, 'choices' | 'failed' | 'open' | 'onOpenChange'> {
  return { name: props.name, unit: props.unit, onAddGenre: props.onAddGenre, onRemoveRef: props.onRemoveRef }
}

function genreLabel(genre: string): string {
  if (genre === '') return 'No genre filter'
  return genre
}

/** A chip per ref, in tab order, sharing one row with the Genre button. */
function GenreChips({ props, stale }: { props: GenreSplitProps; stale: string[] }) {
  const container = genreContainer(props.folderKey, props.group.catalogID)
  const refs = props.group.refs
  const data: ReorderableData = { container, ids: refs.map((ref) => ref.key), reorder: props.onReorder }
  return (
    <ul className="contents">
      <SortableContext items={data.ids.map((key) => dragID(container, key))} strategy={rectSortingStrategy}>
        {refs.map((ref) => (
          <GenreChip
            key={ref.key}
            genreRef={ref}
            data={data}
            movable={refs.length > 1}
            stale={stale.includes(ref.genre)}
            unit={props.unit}
            onRemove={props.onRemoveRef}
          />
        ))}
      </SortableContext>
    </ul>
  )
}

function staleGenres(choices: GenreChoice[]): string[] {
  const genres: string[] = []
  for (const choice of choices) {
    if (choice.stale && choice.refKey !== undefined) genres.push(choice.genre)
  }
  return genres
}

interface GenreChipProps {
  genreRef: FolderRefState
  data: ReorderableData
  /** False for a catalog's only ref, which has nothing to trade places
   *  with and leaves only with the catalog. */
  movable: boolean
  stale: boolean
  unit: FolderUnit
  onRemove(refKey: string): void
}

/** One ref of the catalog: a tab (or row) in Nuvio, dragged by its name. */
function GenreChip({ genreRef, data, movable, stale, unit, onRemove }: GenreChipProps) {
  const sortable = useSortable({ id: dragID(data.container, genreRef.key), data, disabled: !movable })
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = sortable
  const label = genreLabel(genreRef.genre)
  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`bg-raised-hi inline-flex h-7 items-center rounded-full border text-[13px] font-semibold ${chipTone(stale)} ${isDragging ? 'relative z-10 opacity-40' : ''}`}
    >
      <button
        type="button"
        aria-label={`Reorder ${label}`}
        disabled={!movable}
        className="flex h-full cursor-grab touch-none items-center gap-1 rounded-full pr-1.5 pl-2 active:cursor-grabbing disabled:cursor-default pointer-coarse:touch-manipulation"
        {...attributes}
        {...listeners}
      >
        {movable && <GripIcon size={12} className="text-dimmer" />}
        {label}
      </button>
      {movable && (
        <button
          type="button"
          aria-label={`Remove the ${label} ${unit}`}
          onClick={() => onRemove(genreRef.key)}
          className="text-dimmer hover:bg-line hover:text-ink mr-1 grid h-5 w-5 place-items-center rounded-full"
        >
          <Icon icon={X} size={12} />
        </button>
      )}
    </li>
  )
}

function chipTone(stale: boolean): string {
  if (stale) return 'border-danger/60 text-danger'
  return 'border-line-hi text-ink'
}

/**
 * The catalog's genres to tick: no genre filter, then the recipe's own genre
 * options, each tick one tab (or row) in Nuvio. Ticking adds a ref at the end
 * of the catalog's; unticking takes it out. The last tick can't be cleared —
 * a catalog leaves the folder whole, from its "⋯" or the Add catalogs
 * dropdown.
 */
function GenreDropdown({ dropdown, children }: { dropdown: DropdownProps; children: ReactNode }) {
  const ticked = dropdown.choices.filter((choice) => choice.refKey !== undefined).length
  return (
    <Popover.Root open={dropdown.open} onOpenChange={dropdown.onOpenChange}>
      <Popover.Trigger asChild>{children}</Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          align="start"
          sideOffset={6}
          collisionPadding={12}
          aria-label={`${dropdown.name} by genre`}
          className="border-line-hi bg-raised-hi z-40 flex max-h-80 w-[min(20rem,calc(100vw-1.5rem))] flex-col gap-1 overflow-y-auto rounded-xl border p-2"
        >
          <span className="type-label text-dim px-2 pt-1 pb-1.5">One {dropdown.unit} per genre</span>
          <ul className="m-0 flex list-none flex-col gap-0.5 p-0">
            {dropdown.choices.map((choice) => (
              <GenreTick key={choice.genre} choice={choice} locked={ticked === 1} dropdown={dropdown} />
            ))}
          </ul>
          {dropdown.failed && <p className="ed-note m-0 px-2 py-1">Couldn't load this catalog's genres.</p>}
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  )
}

function GenreTick({ choice, locked, dropdown }: { choice: GenreChoice; locked: boolean; dropdown: DropdownProps }) {
  const ticked = choice.refKey !== undefined
  function toggle() {
    if (choice.refKey !== undefined) dropdown.onRemoveRef(choice.refKey)
    else dropdown.onAddGenre(choice.genre)
  }
  return (
    <li className={choice.genre === '' ? 'border-line-hi mb-1 border-b pb-1' : undefined}>
      <label className="hover:bg-line flex cursor-pointer items-center gap-2.5 rounded-lg px-2 py-1.5 text-[14px] has-[:disabled]:cursor-default pointer-coarse:min-h-11">
        <input type="checkbox" checked={ticked} disabled={ticked && locked} onChange={toggle} className="checkbox shrink-0" />
        <span className={choice.stale ? 'text-danger' : undefined}>{genreLabel(choice.genre)}</span>
        {choice.stale && <span className="text-dimmer ml-auto text-[12.5px]">no longer applies</span>}
      </label>
    </li>
  )
}
