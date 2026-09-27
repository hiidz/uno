import { useMemo, useState } from 'react'
import { Copy, Plus } from 'lucide-react'
import { InfoTip } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { filterRefOptions, type RefOption } from './refs'

/**
 * Add catalogs to a folder: search, then click as many as you want.
 *
 * Inline under the folder it fills rather than a dialog: a dialog would put
 * the folder being edited behind a scrim — the one thing the user needs to
 * see while choosing what goes in it.
 *
 * It stays open after each pick and drops each chosen row out of the list, so
 * the remaining options are always exactly what can still be added. Ordering
 * isn't set here: a new ref lands at the end of the folder and is dragged into
 * place in the list above, where the order is visible.
 *
 * Two ways to add a listed catalog, per row: the plus links it (the same
 * catalog everywhere it's used), and a second icon copies it into a fresh,
 * scoped catalog this collection alone references. The third source, "new
 * inside this collection", isn't a pick from this list at all: it's the
 * folder's own New catalog button.
 */
export function CatalogRefPicker({
  options,
  exclude,
  onAdd,
  onCopy,
  onClose,
}: {
  options: RefOption[]
  /** Ids already in *this* folder — omitted, not disabled. See
   *  `filterRefOptions` for why a repeat has to be unrepresentable. */
  exclude: ReadonlySet<string>
  onAdd: (catalogID: string) => void
  onCopy: (catalogID: string) => void
  onClose: () => void
}) {
  const [query, setQuery] = useState('')

  const matches = useMemo(
    () => filterRefOptions(options, query, exclude),
    [options, query, exclude],
  )

  return (
    <div className="border-line-hi bg-raised flex flex-col gap-3 border p-3">
      <div className="flex items-center gap-2">
        <input
          type="search"
          autoFocus
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Search catalogs…"
          aria-label="Search catalogs to add to this folder"
          className="field w-full min-w-0"
        />
        <InfoTip
          label="Adding catalogs"
          text="+ links a catalog, so edits to it show up everywhere it's used. The copy button puts a copy in this collection only."
        />
        <button type="button" onClick={onClose} className="btn-ghost shrink-0">
          Done
        </button>
      </div>

      {matches.length === 0 ? (
        <p className="ed-note m-0">
          {options.length === 0
            ? 'No catalogs yet. Create one in the Library first.'
            : query.trim()
              ? 'No catalogs match this search.'
              : 'Everything available is already in this folder.'}
        </p>
      ) : (
        // Every value visible as its own choice button, DESIGN.md's
        // "copy-from-library choice list, each a plus icon and a catalog
        // name" — wrapping, not a scroll region: the search above already
        // narrows the list to what's worth scanning.
        <div className="choices" role="group" aria-label="Catalogs to add">
          {matches.map((option) => (
            <span key={option.id} className="inline-flex items-center gap-0.5">
              <button
                type="button"
                onClick={() => onAdd(option.id)}
                title={`Link ${option.name} — ${option.recipe} · ${option.catalog.type}`}
                className="choice"
              >
                <Icon icon={Plus} size={13} />
                {option.name}
                <span className="sr-only">to this folder</span>
              </button>
              <button
                type="button"
                onClick={() => onCopy(option.id)}
                title={`Copy ${option.name} into this collection, instead of linking it`}
                aria-label={`Copy ${option.name} into this collection`}
                className="choice px-1.5"
              >
                <Icon icon={Copy} size={12} />
              </button>
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
