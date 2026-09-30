import { useState } from 'react'
import { Copy, Plus, Trash2 } from 'lucide-react'
import { GlyphButton } from '@/components/GlyphButton'
import { Icon } from '@/components/Icon'
import { stickerWords, type SharingSticker } from '@/features/sharing/sharingState'
import { SharingStickers } from '@/features/sharing/SharingStickers'
import { deleteButton } from '@/features/home/deleteBlockers'
import { prefersReducedMotion } from '@/lib/motion'
import { DeleteBlockedNote } from './DeleteBlockedNote'
import { typeLabel } from './recipe'

type RowKind = 'movie' | 'series' | 'collection'

const KIND_LABEL: Record<RowKind, string> = {
  movie: 'Movie catalog',
  series: 'Series catalog',
  collection: 'Collection',
}

/**
 * One row in the Library rail, carrying three separate intentions.
 *
 * **The row body opens it in the pane** for editing. Selecting also reveals
 * the row's actions — duplicating and deleting — so they belong to whichever
 * single row the pane is showing. **The ON NUVIO sticker** puts it on the home
 * screen or takes it off; it stops the click from reaching the row, because
 * adding something to home is not a request to edit it. Putting a row on the
 * home screen from here slaps the sticker on (`data-slap`, `uno-slap` in `index.css`),
 * unless the viewer has asked for less motion; a row that arrives on the home
 * screen some other way just shows it.
 *
 * The actions are `lg` and up only. There the rail sits beside the pane and the
 * selected row stays in view for as long as its editor is open. Below `lg` the
 * two are stacked and selecting a row scrolls the page away from it, so the
 * actions would be a screen-length scroll from the thing they act on; the
 * editor's own sticky header carries them there instead.
 *
 * Every row in the rail is yours, so the stickers left to state are about
 * Community (`rowStickers`): Shared, solid pink because Community is where a
 * shared row turns up, and Changed once it has been edited since; or From
 * Community, outlined pink, on a copy taken from there, with Update or No
 * longer shared beside it.
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
  deleteBlocked = null,
}: {
  kind: RowKind
  /** What the row's sharing state says, in words (`rowStickers`). */
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
  /** Why Delete is disabled — Nuvio may still hold the row — shown beside
   *  it; `null` while it can be deleted. */
  deleteBlocked?: string | null
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
          <span aria-hidden="true" className="mt-0.5 flex flex-wrap gap-1.5">
            <span className="stk stk-kind">{kind === 'collection' ? 'Collection' : typeLabel(kind)}</span>
            <SharingStickers stickers={stickers} />
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
          <GlyphButton
            {...deleteButton(name, deleteBlocked)}
            icon={Trash2}
            onClick={onDelete}
            destructive
            variant="labeled"
          />
          <DeleteBlockedNote reason={deleteBlocked} className="w-full pt-0.5" />
        </div>
      )}
    </div>
  )
}
