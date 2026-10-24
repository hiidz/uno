import type { CatalogType } from '@/api'
import { TMDB_LISTS } from './lists'

/**
 * The TMDB list a catalog shows, picked from tiles: each tile the list's name
 * over one line, the picked one filled. Two a row in the New catalog dialog;
 * `wide` gives the editor three a row from `sm`. `invalid` draws every tile's
 * edge red, for a Create or Save with none picked.
 */
export function ListTiles({
  type,
  value,
  onChange,
  invalid,
  wide = false,
}: {
  type: CatalogType
  value: string | undefined
  onChange: (list: string) => void
  invalid?: boolean
  wide?: boolean
}) {
  return (
    <div
      role="group"
      aria-label="List"
      className={`grid w-full grid-cols-2 gap-2 ${wide ? 'sm:grid-cols-3 max-w-[54rem]' : ''}`}
    >
      {TMDB_LISTS[type].map((list) => {
        const picked = list.value === value
        return (
          // drift-ok: a list tile, the whole tile its target
          <button
            key={list.value}
            type="button"
            aria-pressed={picked}
            onClick={() => onChange(list.value)}
            className={`flex flex-col gap-1 rounded-xl border px-3 py-2.5 text-left transition-colors sm:px-4 ${
              picked
                ? 'bg-ink text-sign-ink border-transparent'
                : `text-ink hover:border-dim ${invalid ? 'border-danger' : 'border-line-hi'}`
            }`}
          >
            <span className="text-meta font-semibold">{list.label}</span>
            <span className={`text-caption ${picked ? 'text-sign-ink' : 'text-dim'}`}>{list.line}</span>
          </button>
        )
      })}
    </div>
  )
}
