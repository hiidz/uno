import { useMemo, useState } from 'react'
import { ChevronDown, Plus } from 'lucide-react'
import { Popover } from 'radix-ui'
import { Icon } from '@/components/Icon'
import { typeLabel } from '@/features/library/recipe'
import { hasFinePointer } from '@/lib/pointer'
import { filterRefOptions, type RefOption } from './refs'

const NOTHING: ReadonlySet<string> = new Set()

/**
 * Add catalogs to a folder: a dropdown under the Add catalogs button with a
 * search field, a New catalog row, then a tick for each library catalog.
 *
 * A tick is the folder itself, not a choice waiting for a button: ticking a
 * catalog adds it to the end of the folder at once, and unticking takes it
 * out, while the dropdown stays open for the next one. Nothing reaches the
 * server until the collection's Save, so there is no Add or Cancel; Escape or
 * a click outside just closes it. A catalog ticked here is the library one,
 * linked: a folder row's own menu is where it becomes one only this
 * collection has (Unlink from library).
 *
 * The ticks track the folder's *unfiltered* refs only, the kind this adds: a
 * catalog that is here only narrowed to a genre shows unticked, and ticking
 * it adds the unfiltered row beside those.
 *
 * New catalog names a new catalog for this folder; when the search matches
 * nothing, it carries the query as the name.
 */
export function CatalogRefPicker({
  options,
  inFolder,
  onAdd,
  onRemove,
  onNew,
}: {
  options: RefOption[]
  /** Ids this folder already holds unfiltered: the ticked ones. */
  inFolder: ReadonlySet<string>
  /** An unfiltered ref to `catalogID`, at the end of the folder. */
  onAdd(catalogID: string): void
  /** The folder's unfiltered ref to `catalogID` taken out. */
  onRemove(catalogID: string): void
  /** A new catalog, named `name`, in this folder. */
  onNew(name: string): void
}) {
  const [open, setOpen] = useState(false)

  function startNew(name: string) {
    setOpen(false)
    onNew(name)
  }

  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger asChild>
        <button type="button" className="btn-ghost btn-sm">
          Add catalogs
          <Icon icon={ChevronDown} size={14} />
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          align="end"
          sideOffset={6}
          collisionPadding={12}
          aria-label="Add catalogs to this folder"
          onOpenAutoFocus={keepKeyboardShut}
          className="border-line-hi bg-raised-hi z-40 flex w-[min(24rem,calc(100vw-1.5rem))] flex-col gap-2 rounded-xl border p-3"
        >
          <PickerBody options={options} inFolder={inFolder} onAdd={onAdd} onRemove={onRemove} onNew={startNew} />
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  )
}

/** The popover's own focus on opening, which lands on the search: kept under
 *  a fine pointer, skipped on touch so a phone opens on the list rather than
 *  under its keyboard. */
function keepKeyboardShut(event: Event) {
  if (!hasFinePointer()) event.preventDefault()
}

interface PickerBodyProps {
  options: RefOption[]
  inFolder: ReadonlySet<string>
  onAdd(catalogID: string): void
  onRemove(catalogID: string): void
  onNew(name: string): void
}

/** The search, New catalog and the ticks, holding one opening's query: it is
 *  mounted only while the dropdown is open. */
function PickerBody({ options, inFolder, onAdd, onRemove, onNew }: PickerBodyProps) {
  const [query, setQuery] = useState('')
  const matches = useMemo(() => filterRefOptions(options, query, NOTHING), [options, query])
  const trimmed = query.trim()

  function addOrRemove(id: string) {
    if (inFolder.has(id)) onRemove(id)
    else onAdd(id)
  }

  return (
    <>
      <input
        type="search"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        placeholder="Search catalogs…"
        aria-label="Search catalogs to add to this folder"
        className="field w-full min-w-0"
      />
      <NewRow name={newName(trimmed, matches.length)} onNew={onNew} />
      <div className="bg-line-hi -mx-3 h-px" />
      <PickerList matches={matches} hasOptions={options.length > 0} inFolder={inFolder} onToggle={addOrRemove} />
    </>
  )
}

/** The name New catalog carries: the search, once it matches nothing. */
function newName(query: string, matchCount: number): string {
  if (matchCount > 0) return ''
  return query
}

/** New catalog, as the list's first row. */
function NewRow({ name, onNew }: { name: string; onNew(name: string): void }) {
  return (
    <button
      type="button"
      onClick={() => onNew(name)}
      className="hover:bg-line flex items-center gap-2.5 rounded-lg px-2 py-2 text-left text-[14px] font-semibold pointer-coarse:min-h-11"
    >
      <Icon icon={Plus} size={16} className="text-collection shrink-0" />
      {newLabel(name)}
    </button>
  )
}

function newLabel(name: string): string {
  if (name === '') return 'New catalog'
  return `New catalog “${name}”`
}

interface PickerListProps {
  matches: RefOption[]
  hasOptions: boolean
  inFolder: ReadonlySet<string>
  onToggle(id: string): void
}

/** A tick row for each catalog the search matches, ticked when the folder
 *  holds it, or a line saying why there are none. */
function PickerList({ matches, hasOptions, inFolder, onToggle }: PickerListProps) {
  if (matches.length === 0) return <p className="ed-note m-0 px-2 py-1">{emptyText(hasOptions)}</p>
  return (
    <ul className="m-0 flex max-h-64 list-none flex-col gap-0.5 overflow-y-auto p-0">
      {matches.map((option) => (
        <li key={option.id}>
          <label
            title={`${option.name} — ${option.recipe}`}
            className="hover:bg-line flex cursor-pointer items-center gap-2.5 rounded-lg px-2 py-2 text-[14px] pointer-coarse:min-h-11"
          >
            <input
              type="checkbox"
              checked={inFolder.has(option.id)}
              onChange={() => onToggle(option.id)}
              className="checkbox shrink-0"
            />
            <span className="min-w-0 flex-1 truncate">{option.name}</span>
            <span className="text-dimmer shrink-0 text-[12.5px]">{typeLabel(option.catalog.type)}</span>
          </label>
        </li>
      ))}
    </ul>
  )
}

function emptyText(hasOptions: boolean): string {
  if (hasOptions) return 'No catalogs match this search.'
  return 'No catalogs in your library yet.'
}
