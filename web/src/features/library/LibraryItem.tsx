import { useState } from 'react'
import { Copy, Plus, Trash2 } from 'lucide-react'
import { GlyphButton } from '@/components/GlyphButton'
import { Icon } from '@/components/Icon'
import { kindSticker, stickerWords, type SharingSticker } from '@/features/sharing/sharingState'
import { SharingStickers } from '@/features/sharing/SharingStickers'
import { prefersReducedMotion } from '@/lib/motion'

type RowKind = 'movie' | 'series' | 'collection'

const KIND_LABEL: Record<RowKind, string> = {
  movie: 'Movie catalog',
  series: 'Series catalog',
  collection: 'Collection',
}

/** The kind sticker a row carries: Movies or Series on a catalog; none on a
 *  collection, since the rail's Collections sign says it. */
const RAIL_KIND: Record<RowKind, SharingSticker[]> = {
  movie: [kindSticker('movie')],
  series: [kindSticker('series')],
  collection: [],
}

/**
 * One row in the Library rail, carrying three separate intentions.
 *
 * **The row body opens it in the pane** for editing, and says whether it is
 * open (`aria-expanded`); from `lg`, pressing the open row closes its editor
 * the way × does. Selecting also reveals the row's actions — duplicating and
 * deleting — so they belong to whichever single row the pane is showing.
 * **The ON NUVIO sticker** puts it on the home
 * screen or takes it off; it stops the click from reaching the row, because
 * adding something to home is not a request to edit it. Putting a row on the
 * home screen from here slaps the sticker on (`data-slap`, `uno-slap` in `index.css`),
 * unless the viewer has asked for less motion; a row that arrives on the home
 * screen some other way just shows it.
 *
 * The actions are `lg` and up only. There the rail sits beside the pane and the
 * selected row stays in view for as long as its editor is open. Below `lg` the
 * row's editor opens as a layer over the rail, so the editor's own header
 * carries them there instead.
 *
 * Every row in the rail is yours, so the one sticker left to state is about
 * Community (`railStickers`), in pink: Published, outlined, then To publish,
 * filled, once it has been edited since; or From Community, outlined, on a row
 * added from there, then Update available, filled, while its publisher's newer
 * version waits. What waits for a push is the pending count's to say.
 */
export function LibraryItem({
  kind,
  stickers = [],
  name,
  summary,
  selected,
  onSelect,
  onHome,
  onToggle,
  onDuplicate,
  onDelete,
}: {
  kind: RowKind
  /** What the row's sharing state says, in words (`railStickers`). */
  stickers?: SharingSticker[]
  name: string
  /** What the row returns or holds, in plain words. Empty draws no line. */
  summary: string
  /** Its editor is the pane's current occupant. */
  selected: boolean
  onSelect: () => void
  /** Already on the home screen — the sticker becomes a remove. */
  onHome: boolean
  onToggle: () => void
  onDuplicate: () => void
  onDelete: () => void
}) {
  const [slap, setSlap] = useState(false)

  return (
    <div
      className={`rounded-xl transition-colors ${selected ? 'bg-raised-hi' : 'hover:bg-raised'}`}
    >
      <div
        role="button"
        tabIndex={0}
        onClick={onSelect}
        onKeyDown={(event) => {
          // A key on the home toggle inside the row is the toggle's own press.
          if (event.target !== event.currentTarget) return
          if (event.key !== 'Enter' && event.key !== ' ') return
          event.preventDefault()
          onSelect()
        }}
        aria-current={selected ? 'true' : undefined}
        aria-expanded={selected}
        title={`Edit ${name}`}
        className="grid w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 rounded-xl py-2.5 pr-2 pl-3 text-left focus-visible:outline-2 focus-visible:outline-offset-[-2px]"
      >
        <div className="flex min-w-0 flex-col gap-1">
          <span className="relative truncate text-[15px] font-semibold">
            {name}
            <span className="sr-only">
              {' '}
              — {KIND_LABEL[kind]}
              {stickerWords(stickers)}
            </span>
          </span>
          {summary && <span className="text-dim truncate text-[13px]">{summary}</span>}
          {/* Hidden while empty: a collection row with no Community sticker. */}
          <span aria-hidden="true" className="mt-0.5 flex flex-wrap gap-1.5 empty:hidden">
            <SharingStickers stickers={[...RAIL_KIND[kind], ...stickers]} />
          </span>
        </div>

        <button
          type="button"
          onClick={(event) => {
            // Adding something to the home screen is not a request to edit it,
            // so this must not reach the row's own click.
            event.stopPropagation()
            // Only when the animation will run: its end is what clears the flag.
            setSlap(!onHome && !prefersReducedMotion())
            onToggle()
          }}
          onAnimationEnd={() => setSlap(false)}
          data-slap={slap || undefined}
          aria-pressed={onHome}
          aria-label={
            onHome ? `Take ${name} off your home screen` : `Put ${name} on your home screen`
          }
          title={onHome ? 'On your home screen — click to remove' : 'Add to home screen'}
          className="home-sticker"
        >
          {onHome ? (
            <span aria-hidden="true" className="home-sticker-words">
              <span>ON</span>
              <span>NUVIO</span>
            </span>
          ) : (
            <Icon icon={Plus} size={18} />
          )}
        </button>
      </div>

      {selected && (
        <div className="hidden flex-wrap items-center gap-1.5 px-3 pb-3 lg:flex">
          <GlyphButton label={`Duplicate ${name}`} icon={Copy} onClick={onDuplicate} variant="labeled" />
          <GlyphButton label={`Delete ${name}`} icon={Trash2} onClick={onDelete} destructive variant="labeled" />
        </div>
      )}
    </div>
  )
}
