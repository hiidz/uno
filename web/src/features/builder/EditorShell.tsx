import { useEffect } from 'react'
import type { ReactNode } from 'react'
import { TypeBar, type BarKind } from '@/components/TypeBar'
import { useStackedLayout } from './stacked'

/**
 * The frame around an editor filling the builder's pane.
 *
 * The pane holds one occupant at a time — either the home screen or one
 * editor — so there is no tab strip and no way to express a second open form:
 * the title says what is being edited, and the × closes it.
 *
 * **Closing is a request, not an act.** `onRequestClose` may raise a discard
 * confirmation instead of closing, which is why Escape routes through the same
 * call rather than closing directly — and why the mobile Library button below
 * does too, rather than getting a scroll-only callback of its own.
 *
 * **Below `lg` the header is this editor's only chrome**, and it changes job:
 * pinned under the app header, it is the one thing on screen for as long as the
 * form is, so it carries the library row's own duplicate and delete. Those
 * actions live on the row itself above `lg`, where the rail is beside the pane
 * and the row is a glance away; stacked, the row is a screen-length scroll up,
 * and this is the only surface that stays.
 *
 * The × itself is hidden below `lg` rather than duplicated: **the Library
 * button in that position is the same exit**, not a second one. Tapping it
 * deselects this row exactly as × does above `lg` — same `onRequestClose`, same
 * guard, same landing on home — and the workspace's own scroll request is what
 * then carries the reader up to the rail. It reads as "back" only because
 * deselecting below `lg` and scrolling to the rail happen to land together.
 */
export function EditorShell({
  eyebrow,
  title,
  kind,
  owned,
  onRequestClose,
  onDuplicate,
  onDelete,
  footer,
  children,
}: {
  /** What kind of edit this is — "Edit catalog", "New collection". The title
   *  below it is the subject's own name, which is what the eye lands on.
   *  Dropped below `lg`, where the header is one compact line. */
  eyebrow: string
  title: string
  /** Carries the rail row's hue into the pane, so the thing you clicked and the
   *  thing in front of you are visibly the same thing. */
  kind: BarKind
  owned: boolean
  /** Above `lg` this is the × in this header and Escape. Below it, it's also
   *  the mobile Library button — there is no separate scroll-only path back to
   *  the rail here, because leaving this row *is* an exit: it deselects, which
   *  a dirty form has to be able to hold and ask about. */
  onRequestClose: () => void
  /** The open row's own actions, mirrored here below `lg`. Absent when there is
   *  no row to act on — a catalog being created, or a community row, which can
   *  be neither duplicated from here nor deleted. */
  onDuplicate?: () => void
  onDelete?: () => void
  footer: ReactNode
  children: ReactNode
}) {
  const stacked = useStackedLayout()

  // Escape closes, matching the convention every modal dialog sets — the
  // muscle memory is already there and there's no reason to take it away.
  // Routed through `onRequestClose` so a dirty form still confirms.
  useEffect(() => {
    // Stacked there is no × for it to be the shortcut *to*, and the pane may
    // not even be what's on screen — emptying it from the rail would be a
    // change with no visible cause.
    if (stacked) return

    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== 'Escape') return
      // A dialog is in front of the editor and Escape belongs to it. `Modal`
      // listens on `document` too, so without this both fire: dismissing a
      // discard prompt would immediately ask again, and dismissing a delete
      // confirmation would also try to close the editor behind it.
      if (document.querySelector('[role="dialog"]')) return
      onRequestClose()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [onRequestClose, stacked])

  return (
    <main className="flex min-w-0 flex-1 flex-col lg:h-full lg:min-h-0">
      {/* Pinned under the app header below `lg` — `--app-h` is that header
          measured, not assumed, because the push banner mounts and unmounts
          beneath it. Above `lg` the flex column already holds this in place and
          `static` restores exactly what was here before. */}
      <header className="border-line bg-ground sticky top-[var(--app-h)] z-20 flex shrink-0 items-center gap-3 border-b px-4 py-3 lg:static lg:bg-transparent lg:px-6 lg:py-4">
        <TypeBar kind={kind} owned={owned} className="min-h-[26px] lg:min-h-[30px]" />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <span className="type-eyebrow hidden lg:block">{eyebrow}</span>
          {/* Where focus lands when the page scrolls here. A scroll moves the
              viewport and nothing else, so without this a keyboard or
              screen-reader user is left behind in the rail. */}
          <h1
            tabIndex={-1}
            data-landing
            className="type-display m-0 truncate text-[15px] outline-none lg:text-[17px]"
          >
            {title}
          </h1>
        </div>

        {/* The row's actions and the way out, below `lg` only. */}
        <div className="flex shrink-0 items-center gap-1.5 lg:hidden">
          {onDuplicate && (
            <HeaderAction label={`Duplicate ${title}`} glyph="⧉" onClick={onDuplicate} />
          )}
          {onDelete && (
            <HeaderAction label={`Delete ${title}`} glyph="🗑" onClick={onDelete} destructive />
          )}
          {/* Same call as the desktop ×, not a scroll of its own — see the
              doc comment on `onRequestClose` above. A held discard prompt has
              to be able to stop this exactly as it stops that one. */}
          <button
            type="button"
            onClick={onRequestClose}
            title="Close editor (back to the library)"
            className="tap type-data border-line-hi text-dim hover:text-ink hover:border-dim flex h-7 shrink-0 items-center gap-1 rounded-[2px] border px-2 text-[10px] tracking-[0.06em] uppercase transition-colors"
          >
            <span aria-hidden="true" className="text-[11px] leading-none">
              ↑
            </span>
            Library
          </button>
        </div>

        <button
          type="button"
          onClick={onRequestClose}
          aria-label="Close editor and go back to your home screen"
          title="Close editor (Esc)"
          className="tap text-dim hover:text-ink hover:border-dim ml-auto hidden h-7 w-7 shrink-0 place-items-center rounded-[2px] border border-transparent text-[15px] leading-none transition-colors lg:grid"
        >
          ×
        </button>
      </header>

      {/* One inset on all three bands — `px-4` narrow, `px-6` from `lg`: the
          header, the form, and the footer are stacked and share an edge, so a
          padding that differs between them reads as a misalignment rather than
          as a rhythm. The narrow value is smaller because 24px of gutter on
          each side of a 375px screen is 13% of it.

          `--w-form` caps the form itself, not the pane. The pane is as wide as
          the window allows and the editors are two columns of short controls —
          past about 860px a row stops being something you read across, and
          every field in it starts looking stretched. */}
      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4 lg:px-6">
        <div className="w-full max-w-[var(--w-form)]">{children}</div>
      </div>

      {/* Sticky below `lg`, where the page scrolls as one document and Save
          would otherwise sit at the far end of a form as long as the catalog
          editor. It needs a ground of its own to sit over the form; above `lg`
          the flex column pins it and the band is transparent as before. */}
      <div className="border-line bg-ground sticky bottom-0 z-20 flex shrink-0 flex-wrap items-center justify-end gap-2 border-t px-4 py-4 lg:static lg:bg-transparent lg:px-6">
        {footer}
      </div>
    </main>
  )
}

/** Glyph-only: the header is one line on the screens this renders on, and the
 *  title beside it is what needs the room. */
function HeaderAction({
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
      onClick={onClick}
      aria-label={label}
      title={label}
      className={`tap border-line-hi text-dim grid h-7 w-7 shrink-0 place-items-center rounded-[2px] border text-[12px] leading-none transition-colors ${
        destructive ? 'hover:border-danger hover:text-danger' : 'hover:border-dim hover:text-ink'
      }`}
    >
      <span aria-hidden="true">{glyph}</span>
    </button>
  )
}
