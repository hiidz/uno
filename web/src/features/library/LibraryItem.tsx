import { useState } from 'react'
import { Copy, Plus, Trash2 } from 'lucide-react'
import { GlyphButton } from '@/components/GlyphButton'
import { Icon } from '@/components/Icon'
import { SkeletonBar } from '@/components/Skeleton'
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
 * screen or takes it off. It is the row button's sibling, not its child,
 * because adding something to home is not a request to edit it. Putting a row on the
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
      {/* Two sibling buttons, never one inside the other. The row's button
          stretches over the whole row through its `::after`, so a press
          anywhere but the sticker opens it, and its focus ring rings the row;
          the sticker sits above that layer. */}
      <div className="relative grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 py-2.5 pr-2 pl-3">
        <button
          type="button"
          onClick={onSelect}
          aria-current={selected ? 'true' : undefined}
          aria-expanded={selected}
          title={name}
          className="flex min-w-0 flex-col gap-1 text-left outline-none after:absolute after:inset-0 after:rounded-xl focus-visible:after:outline-2 focus-visible:after:outline-offset-[-2px]"
        >
          <span className="relative truncate text-[15px] font-semibold">
            {name}
            <span className="sr-only">
              {' '}
              — {KIND_LABEL[kind]}
              {stickerWords(stickers)}
            </span>
          </span>
          {summary && (
            <span title={summary} className="text-dim truncate text-[13px]">
              {summary}
            </span>
          )}
          {/* Hidden while empty: a collection row with no Community sticker. */}
          <span aria-hidden="true" className="mt-0.5 flex flex-wrap gap-1.5 empty:hidden">
            <SharingStickers stickers={[...RAIL_KIND[kind], ...stickers]} />
          </span>
        </button>

        <button
          type="button"
          onClick={() => {
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

/** A row's stand-in while the library loads: `LibraryItem`'s grid and
 *  padding, its name, summary and sticker lines at their own heights, and a
 *  circle where the home sticker goes. */
export function LibraryItemSkeleton() {
  return (
    <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 py-2.5 pr-2 pl-3">
      <div className="flex min-w-0 flex-col gap-1">
        <span className="flex h-[22.5px] items-center">
          <SkeletonBar className="h-3.5 w-2/5" />
        </span>
        <span className="flex h-[19.5px] items-center">
          <SkeletonBar className="h-3 w-4/5" />
        </span>
        <span className="mt-0.5 flex">
          <SkeletonBar className="h-5 w-16 rounded-full" />
        </span>
      </div>
      <SkeletonBar className="size-[42px] rounded-full pointer-coarse:size-11" />
    </div>
  )
}
