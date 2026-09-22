import { GlyphButton } from '@/components/GlyphButton'

export type BarKind = 'movie' | 'series' | 'collection'

const KIND_LABEL: Record<BarKind, string> = {
  movie: 'Movie catalog',
  series: 'Series catalog',
  collection: 'Collection',
}

/**
 * One row in the Library rail, carrying three separate intentions.
 *
 * **The row body opens it in the pane** for editing. Selecting also reveals
 * the row's actions — duplicating and deleting — so they belong to whichever
 * single row the pane is showing. **The toggle** puts it on the home screen
 * or takes it off; it stops the click from reaching the row, because adding
 * something to home is not a request to edit it.
 *
 * The actions are `lg` and up only. There the rail sits beside the pane and the
 * selected row stays in view for as long as its editor is open. Below `lg` the
 * two are stacked and selecting a row scrolls the page away from it, so the
 * actions would be a screen-length scroll from the thing they act on; the
 * editor's own sticky header carries them there instead.
 *
 * `isPublic` marks a row as also shared to the community — the closed-graph
 * model means every row in the rail is yours, so this is the only ownership
 * fact left worth stating.
 */
export function LibraryItem({
  kind,
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
  /** Visible to everyone else, not just you. */
  isPublic?: boolean
  name: string
  summary: string
  /** Its editor is the pane's current occupant. */
  selected: boolean
  onSelect: () => void
  /** Already on the home screen — the button becomes a remove. */
  onHome: boolean
  onToggle: () => void
  onDuplicate?: () => void
  onDelete?: () => void
}) {
  const hasActions = Boolean(onDuplicate || onDelete)

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
          // A key on the home toggle inside the row is the toggle's own press.
          if (event.target !== event.currentTarget) return
          if (event.key !== 'Enter' && event.key !== ' ') return
          event.preventDefault()
          onSelect()
        }}
        aria-current={selected ? 'true' : undefined}
        title={`Edit ${name}`}
        className={`grid w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 py-2.5 pr-2 text-left transition-colors ${
          selected ? '' : 'hover:bg-raised'
        } ${selected ? 'pl-2' : ''}`}
      >
        <div className="flex min-w-0 flex-col gap-[3px]">
          <span className={`truncate text-[13px] ${selected ? 'font-semibold' : 'font-medium'}`}>
            {name}
            <span className="sr-only">
              {' '}
              — {KIND_LABEL[kind]}
              {isPublic ? ', shared with the community' : ''}
            </span>
          </span>
          <span className="type-data text-dimmer flex min-w-0 items-baseline gap-1.5 text-[10.5px]">
            <span className="truncate">{summary}</span>
            {isPublic && (
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
          className={`tap type-data grid h-6 w-6 shrink-0 place-items-center rounded-[2px] border text-[11px] leading-none transition-colors pointer-coarse:h-9 pointer-coarse:w-9 ${
            onHome
              ? 'bg-dim border-dim text-ground hover:bg-danger hover:border-danger hover:text-white'
              : 'border-line-hi text-dim hover:text-ink hover:border-dim'
          }`}
        >
          {onHome ? '✓' : '+'}
        </button>
      </div>

      {hasActions && selected && (
        <div className="hidden items-center gap-1.5 px-2 pb-2.5 pl-2 lg:flex">
          {onDuplicate && (
            <GlyphButton
              label={`Duplicate ${name}`}
              glyph="⧉"
              onClick={onDuplicate}
              variant="labeled"
            />
          )}
          {onDelete && (
            <GlyphButton
              label={`Delete ${name}`}
              glyph="🗑"
              onClick={onDelete}
              destructive
              variant="labeled"
            />
          )}
        </div>
      )}
    </div>
  )
}
