import { useId, useState } from 'react'
import { ChevronRight } from 'lucide-react'
import type { Catalog } from '@/api'
import { Icon } from '@/components/Icon'
import { recipeFacts, recipeLine, type RecipeFact } from '@/features/library/recipe'
import type { GenreLookups } from '@/features/library/useLibrary'

/** The genre lookup for a catalog's own kind: movie and tv ids differ. */
function lookupFor(catalog: Catalog, genres: GenreLookups) {
  return catalog.type === 'movie' ? genres.movie : genres.tv
}

/** A catalog's facts as spec tiles, two to a row: a dim label over a bold
 *  value. */
function FactTiles({ facts }: { facts: RecipeFact[] }) {
  return (
    <dl className="m-0 grid grid-cols-2 gap-2">
      {facts.map((fact) => (
        <div key={fact.label} className="bg-raised-hi flex min-w-0 flex-col gap-1 rounded-[10px] px-3 py-2.5">
          <dt className="text-dim text-[12px] font-semibold">{fact.label}</dt>
          <dd className="m-0 text-[15px] font-bold [overflow-wrap:anywhere] tabular-nums">{fact.value}</dd>
        </div>
      ))}
    </dl>
  )
}

/**
 * A catalog block, the one way a catalog added from Community is read: in its
 * view, in a collection's folders, and on a publication's page.
 *
 * Open, it is the recipe's spec tiles and nothing else; the name is the
 * surrounding page's to show. With `foldable` it is a folder's entry instead:
 * collapsed it is a chevron, the catalog's name and its recipe line under it,
 * and the header opens it in place to the same tiles.
 */
export function CatalogBlock({
  catalog,
  genres,
  narrowedTo = '',
  foldable = false,
}: {
  catalog: Catalog
  genres: GenreLookups
  /** The genre a folder narrows this catalog to, when it does. */
  narrowedTo?: string
  foldable?: boolean
}) {
  const [open, setOpen] = useState(false)
  const panelID = useId()
  const lookup = lookupFor(catalog, genres)
  const facts = recipeFacts(catalog, lookup)
  const tiles = <FactTiles facts={narrowedTo ? [...facts, { label: 'Narrowed to', value: narrowedTo }] : facts} />
  if (!foldable) return tiles

  const line = recipeLine(catalog, lookup)
  return (
    <div className="border-line border-t py-2 first:border-t-0">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={panelID}
        onClick={() => setOpen(!open)}
        className="tap flex w-full items-start gap-2.5 py-1 text-left"
      >
        <Icon icon={ChevronRight} size={16} className={`text-dimmer mt-0.5 shrink-0 transition-transform ${open ? 'rotate-90' : ''}`} />
        <span className="flex min-w-0 flex-col gap-0.5">
          <span className="text-[14px] font-semibold">{catalog.name}</span>
          {!open && (
            <span className="text-dim text-[12.5px]">{narrowedTo ? `${line} • ${narrowedTo}` : line}</span>
          )}
        </span>
      </button>
      <div id={panelID} hidden={!open} className="pt-2 pl-[26px]">
        {open && tiles}
      </div>
    </div>
  )
}
