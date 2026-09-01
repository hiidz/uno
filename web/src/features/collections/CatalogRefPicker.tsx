import { useMemo, useState } from 'react'
import { TypeBar } from '@/components/TypeBar'
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
    <div className="border-line-hi bg-raised mt-2 flex flex-col gap-3 border p-3">
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
        <button type="button" onClick={onClose} className="btn-ghost shrink-0">
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
        // The cap is lifted below `lg`: an inner scroll region inside a page
        // that already scrolls is a box a thumb gets stuck in, and the list is
        // already narrowed by the search above it.
        <ul className="m-0 flex max-h-none max-w-[42rem] list-none flex-col overflow-y-auto overscroll-contain p-0 lg:max-h-[210px]">
          {matches.map((option) => (
            <li key={option.id}>
              <button
                type="button"
                onClick={() => onAdd(option.id)}
                className="border-line hover:bg-raised-hi grid w-full grid-cols-[3px_minmax(0,1fr)_20px] items-center gap-x-3 border-b py-2 text-left transition-colors last:border-b-0 sm:grid-cols-[3px_minmax(0,1fr)_7rem_20px]"
              >
                <TypeBar
                  kind={option.catalog.type}
                  owned={option.catalog.owned}
                  className="min-h-[22px]"
                />
                <span className="flex min-w-0 flex-col gap-0.5">
                  <span className="truncate text-[12.5px]">{option.name}</span>
                  <span className="type-data text-dimmer truncate text-[10.5px]">
                    {option.recipe}
                  </span>
                  {/* Below `sm` the column to the right is gone and these words
                      move under the recipe — see `RefRow`, which drops the same
                      column at the same width. */}
                  <span className="type-data text-dimmer truncate text-[10.5px] sm:hidden">
                    {option.catalog.type === 'movie' ? 'movie' : 'series'} ·{' '}
                    {option.catalog.owned ? 'you' : 'community'}
                  </span>
                </span>
                {/* The type bar carries kind and ownership visually; it's
                    aria-hidden, so both facts have to exist as text too. */}
                <span className="type-data text-dimmer hidden truncate text-[10.5px] sm:block">
                  {option.catalog.type === 'movie' ? 'movie' : 'series'} ·{' '}
                  {option.catalog.owned ? 'you' : 'community'}
                </span>
                <span aria-hidden="true" className="text-dim text-center text-[13px]">
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
