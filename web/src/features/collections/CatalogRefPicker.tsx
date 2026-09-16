import { useMemo, useState } from 'react'
import { Plus } from 'lucide-react'
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
 */
export function CatalogRefPicker({
  options,
  exclude,
  onAdd,
  onClose,
}: {
  options: RefOption[]
  /** Ids already in *this* folder — omitted, not disabled. See
   *  `filterRefOptions` for why a repeat has to be unrepresentable. */
  exclude: ReadonlySet<string>
  onAdd: (catalogID: string) => void
  onClose: () => void
}) {
  const [query, setQuery] = useState('')

  const matches = useMemo(
    () => filterRefOptions(options, query, exclude),
    [options, query, exclude],
  )

  return (
    <div className="border-line-hi bg-raised mt-1 flex flex-col gap-3 border p-3">
      <div className="flex items-center gap-2">
        <input
          type="search"
          autoFocus
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Search catalogs…"
          aria-label="Search catalogs to add to this folder"
          className="field type-data w-full max-w-[var(--w-entry)] text-[12.5px] pointer-coarse:text-[16px]"
        />
        <button type="button" onClick={onClose} className="btn-quiet shrink-0">
          Done
        </button>
      </div>

      {matches.length === 0 ? (
        <p className="type-data text-dimmer m-0 px-1 py-1.5 text-[11px]">
          {options.length === 0
            ? 'No catalogs yet. Create one in the sidebar first.'
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
            <button
              key={option.id}
              type="button"
              onClick={() => onAdd(option.id)}
              title={`${option.recipe} · ${option.catalog.type === 'movie' ? 'movie' : 'series'} · ${
                option.catalog.owned ? 'yours' : 'community'
              }`}
              className="choice"
            >
              <Icon icon={Plus} size={13} />
              {option.name}
              <span className="sr-only">to this folder</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
