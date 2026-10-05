import { useMemo, useState } from 'react'
import { ChevronDown, Plus } from 'lucide-react'
import { Popover } from 'radix-ui'
import { Icon } from '@/components/Icon'
import { typeLabel } from '@/features/library/recipe'
import { pluralCount } from '@/lib/plural'
import { filterRefOptions, type RefOption } from './refs'

/**
 * Add catalogs to a folder: a dropdown under the Add catalogs button, a search
 * field, then a tick for each catalog the folder doesn't hold yet.
 *
 * The ticks are the dropdown's own state. "Add N catalogs" links them all,
 * in library order, and closes it; Cancel, Escape or a click outside adds
 * nothing. Ordering isn't set here: a new ref lands at the end of the folder
 * and is dragged into place in the list below, where the order is visible.
 *
 * A search that matches nothing offers to name a new catalog after it, which
 * is the folder's own New catalog button with the query already typed.
 */
export function CatalogRefPicker({
  options,
  exclude,
  onAdd,
  onNew,
}: {
  options: RefOption[]
  /** Ids already in *this* folder — omitted, not disabled. See
   *  `filterRefOptions` for why a repeat has to be unrepresentable. */
  exclude: ReadonlySet<string>
  /** The ticked catalogs' ids, in library order. */
  onAdd: (catalogIDs: string[]) => void
  /** A new catalog, named `name`, in this folder. */
  onNew: (name: string) => void
}) {
  const [open, setOpen] = useState(false)

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
          className="border-line-hi bg-raised-hi z-40 flex w-[min(24rem,calc(100vw-1.5rem))] flex-col gap-3 rounded-xl border p-3"
        >
          <PickerBody
            options={options}
            exclude={exclude}
            onAdd={(ids) => {
              onAdd(ids)
              setOpen(false)
            }}
            onNew={(name) => {
              setOpen(false)
              onNew(name)
            }}
            onCancel={() => setOpen(false)}
          />
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  )
}

/** The search, the ticks and the buttons, holding one opening's choices: it
 *  is mounted only while the dropdown is open. */
function PickerBody({
  options,
  exclude,
  onAdd,
  onNew,
  onCancel,
}: {
  options: RefOption[]
  exclude: ReadonlySet<string>
  onAdd: (catalogIDs: string[]) => void
  onNew: (name: string) => void
  onCancel: () => void
}) {
  const [query, setQuery] = useState('')
  const [ticked, setTicked] = useState<ReadonlySet<string>>(new Set())

  const matches = useMemo(() => filterRefOptions(options, query, exclude), [options, query, exclude])
  const count = ticked.size

  function toggle(id: string) {
    setTicked((current) => {
      const next = new Set(current)
      if (!next.delete(id)) next.add(id)
      return next
    })
  }

  return (
    <>
      <input
        type="search"
        autoFocus
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        placeholder="Search catalogs…"
        aria-label="Search catalogs to add to this folder"
        className="field w-full min-w-0"
      />

      {matches.length === 0 ? (
        <EmptyList hasOptions={options.length > 0} query={query.trim()} onNew={onNew} />
      ) : (
        <ul className="m-0 flex max-h-64 list-none flex-col gap-0.5 overflow-y-auto p-0">
          {matches.map((option) => (
            <li key={option.id}>
              <label
                title={`${option.name} — ${option.recipe}`}
                className="hover:bg-line flex cursor-pointer items-center gap-2.5 rounded-lg px-2 py-1.5 text-[14px]"
              >
                <input
                  type="checkbox"
                  checked={ticked.has(option.id)}
                  onChange={() => toggle(option.id)}
                  className="shrink-0"
                />
                <span className="min-w-0 flex-1 truncate">{option.name}</span>
                <span className="text-dimmer shrink-0 text-[12.5px]">{typeLabel(option.catalog.type)}</span>
              </label>
            </li>
          ))}
        </ul>
      )}

      <div className="flex items-center justify-end gap-2">
        <button type="button" onClick={onCancel} className="btn-ghost btn-sm">
          Cancel
        </button>
        <button
          type="button"
          disabled={count === 0}
          onClick={() => onAdd(options.filter((option) => ticked.has(option.id)).map((option) => option.id))}
          className="btn-primary btn-sm"
        >
          {count === 0 ? 'Add catalogs' : `Add ${pluralCount(count, 'catalog')}`}
        </button>
      </div>
    </>
  )
}

function EmptyList({
  hasOptions,
  query,
  onNew,
}: {
  hasOptions: boolean
  query: string
  onNew: (name: string) => void
}) {
  if (query) {
    return (
      <div className="flex flex-col items-start gap-2">
        <p className="ed-note m-0">No catalogs match this search.</p>
        <button type="button" onClick={() => onNew(query)} className="btn-secondary btn-sm">
          <Icon icon={Plus} size={13} />
          New catalog “{query}”
        </button>
      </div>
    )
  }
  return (
    <p className="ed-note m-0">
      {hasOptions ? 'Everything available is already in this folder.' : 'No catalogs yet. Create one with New catalog.'}
    </p>
  )
}
