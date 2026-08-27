import { useMemo, useState } from 'react'
import { TypeBar } from '@/components/TypeBar'
import { filterRefOptions, type RefOption } from './refs'

/**
 * Add catalogs to a folder: search, then click as many as you want.
 *
 * Inline under the folder it fills rather than a dialog. The builder is already
 * a dialog over the Manage overlay, and a third layer would put the folder
 * being edited behind two scrims — the one thing the user needs to see while
 * choosing what goes in it.
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
    <div className="border-line-hi bg-raised mt-2 flex flex-col gap-2 border p-2.5">
      <div className="flex items-center gap-2">
        <input
          type="search"
          autoFocus
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Search catalogs by name or genre…"
          aria-label="Search catalogs to add to this folder"
          className="field type-data w-full text-[11.5px]"
        />
        <button type="button" onClick={onClose} className="btn-ghost shrink-0">
          Done
        </button>
      </div>

      {matches.length === 0 ? (
        <p className="type-data text-dimmer m-0 px-1 py-1.5 text-[10.5px]">
          {options.length === 0
            ? 'No catalogs exist yet. Build one under Manage catalogs first.'
            : query.trim()
              ? 'No catalogs match this search.'
              : 'Every catalog you can use is already in this folder.'}
        </p>
      ) : (
        <ul className="m-0 flex max-h-[210px] list-none flex-col overflow-y-auto p-0">
          {matches.map((option) => (
            <li key={option.id}>
              <button
                type="button"
                onClick={() => onAdd(option.id)}
                className="border-line hover:bg-raised-hi flex w-full items-center gap-2.5 border-b py-1.5 pr-1 text-left transition-colors last:border-b-0"
              >
                <TypeBar
                  kind={option.catalog.type}
                  owned={option.catalog.owned}
                  className="min-h-[22px]"
                />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-[12px]">{option.name}</span>
                  <span className="type-data text-dimmer block truncate text-[10px]">
                    {option.recipe}
                  </span>
                </span>
                {/* The type bar carries kind and ownership visually; it's
                    aria-hidden, so both facts have to exist as text too. */}
                <span className="type-data text-dimmer shrink-0 text-[10px]">
                  {option.catalog.type === 'movie' ? 'movie' : 'series'} ·{' '}
                  {option.catalog.owned ? 'you' : 'community'}
                </span>
                <span aria-hidden="true" className="text-dim shrink-0 px-1 text-[13px]">
                  +
                </span>
                <span className="sr-only">Add {option.name} to this folder</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
