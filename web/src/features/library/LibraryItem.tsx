import { TypeBar, type BarKind } from '@/components/TypeBar'

const KIND_LABEL: Record<BarKind, string> = {
  movie: 'Movie catalog',
  series: 'Series catalog',
  collection: 'Collection',
}

/**
 * One row in the Library rail, carrying three separate intentions.
 *
 * **The row body opens it in the pane** — editing yours, or starting your own
 * copy of someone else's, which is the only thing a community row can become.
 * Selecting also reveals the row's actions — duplicating one of your own, and
 * deleting — so they belong to whichever single row the pane is showing.
 * **The toggle** puts it on the home screen or takes it off; it stops the click
 * from reaching the row, because adding something to home is not a request to
 * edit it.
 *
 * The actions are `lg` and up only. There the rail sits beside the pane and the
 * selected row stays in view for as long as its editor is open. Below `lg` the
 * two are stacked and selecting a row scrolls the page away from it, so the
 * actions would be a screen-length scroll from the thing they act on; the
 * editor's own sticky header carries them there instead.
 *
 * The type bar carries kind and ownership visually, but a 3px hatch is easy to
 * miss at rail width, so community rows say so in text too. Kind stays
 * screen-reader-only: it's redundant with the recipe for sighted users but is
 * the bar's only equivalent for everyone else.
 *
 * `isPublic` is a separate axis from `owned`: it marks one of *your own* rows
 * as also shared to the community, rather than which section it lives in.
 * Only meaningful (and only rendered) on owned rows — a community row is
 * public by definition, so saying so again would be noise.
 */
export function LibraryItem({
  kind,
  owned,
  isPublic,
  name,
  summary,
  selected,
  onSelect,
  onHome,
  onToggle,
  onDuplicate,
  onDelete,
}: {
  kind: BarKind
  owned: boolean
  /** Owned rows only: yours, but also visible to everyone else. */
  isPublic?: boolean
  name: string
  summary: string
  /** Its editor is the pane's current occupant. */
  selected: boolean
  onSelect: () => void
  /** Already on the home screen — the button becomes a remove. */
  onHome: boolean
  onToggle: () => void
  /** Owned rows only: a community row's *selection* is already a duplicate. */
  onDuplicate?: () => void
  /** Owned rows only. */
  onDelete?: () => void
}) {
  const hasActions = Boolean(onDuplicate || onDelete)
  const shared = owned && isPublic

  return (
    <div
      className={`border-b border-[color-mix(in_srgb,var(--uno-line)_55%,transparent)] ${
        selected ? 'bg-raised-hi shadow-[inset_2px_0_0_var(--uno-ink)]' : ''
      }`}
    >
      <div
        role="button"
        tabIndex={0}
        onClick={onSelect}
        onKeyDown={(event) => {
          if (event.key !== 'Enter' && event.key !== ' ') return
          event.preventDefault()
          onSelect()
        }}
        aria-current={selected ? 'true' : undefined}
        title={owned ? `Edit ${name}` : `Open ${name} as your own copy`}
        className={`grid w-full grid-cols-[3px_minmax(0,1fr)_auto] items-center gap-x-3 py-2.5 pr-2 text-left transition-colors ${
          selected ? '' : 'hover:bg-raised'
        } ${selected ? 'pl-2' : ''}`}
      >
        <TypeBar kind={kind} owned={owned} className="min-h-[26px]" />
        <div className="flex min-w-0 flex-col gap-[3px]">
          <span className={`truncate text-[13px] ${selected ? 'font-semibold' : 'font-medium'}`}>
            {name}
            <span className="sr-only">
              {' '}
              — {KIND_LABEL[kind]}, {owned ? 'yours' : 'community'}
              {shared ? ', shared with the community' : ''}
            </span>
          </span>
          <span className="type-data text-dimmer flex min-w-0 items-baseline gap-1.5 text-[10.5px]">
            <span className="truncate">{summary}</span>
            {!owned && (
              <span aria-hidden="true" className="shrink-0">
                · community
              </span>
            )}
            {shared && (
              <span
                aria-hidden="true"
                title="Also visible to the community"
                className="text-ink shrink-0"
              >
                · shared
              </span>
            )}
          </span>
        </div>

        <button
          type="button"
          onClick={(event) => {
            // Adding something to the home screen is not a request to edit it,
            // so this must not reach the row's own click.
            event.stopPropagation()
            onToggle()
          }}
          aria-pressed={onHome}
          aria-label={
            onHome ? `Remove ${name} from your home screen` : `Add ${name} to your home screen`
          }
          title={onHome ? 'On your home screen — click to remove' : 'Add to your home screen'}
          className={`tap type-data grid h-6 w-6 shrink-0 place-items-center rounded-[2px] border text-[11px] leading-none transition-colors ${
            onHome
              ? 'bg-dim border-dim text-ground hover:bg-danger hover:border-danger hover:text-white'
              : 'border-line-hi text-dim hover:text-ink hover:border-dim'
          }`}
        >
          {onHome ? '✓' : '+'}
        </button>
      </div>

      {hasActions && selected && (
        <div className="hidden items-center gap-1.5 px-2 pb-2.5 pl-[23px] lg:flex">
          {onDuplicate && <ItemAction label={`Duplicate ${name}`} glyph="⧉" onClick={onDuplicate} />}
          {onDelete && (
            <ItemAction label={`Delete ${name}`} glyph="🗑" onClick={onDelete} destructive />
          )}
        </div>
      )}
    </div>
  )
}

function ItemAction({
  label,
  glyph,
  onClick,
  destructive,
}: {
  label: string
  glyph: string
  onClick: () => void
  destructive?: boolean
}) {
  return (
    <button
      type="button"
      onClick={(event) => {
        event.stopPropagation()
        onClick()
      }}
      aria-label={label}
      title={label}
      className={`type-data flex h-6 items-center gap-1 rounded-[2px] border px-2 text-[10px] tracking-[0.06em] uppercase transition-colors pointer-coarse:h-9 ${
        destructive
          ? 'border-line-hi text-dim hover:border-danger hover:text-danger'
          : 'border-line-hi text-dim hover:border-dim hover:text-ink'
      }`}
    >
      <span aria-hidden="true" className="text-[11px] leading-none">
        {glyph}
      </span>
      {label.split(' ')[0]}
    </button>
  )
}
