import { useEffect } from 'react'
import type { ReactNode } from 'react'
import { Copy, Trash2, X } from 'lucide-react'
import { GlyphButton } from '@/components/GlyphButton'
import { Icon } from '@/components/Icon'
import { PaneSign, SIGN_TITLE, SignLibraryButton, SignStepButton, type SignStep } from '@/components/PaneSign'
import { useStackedLayout } from './stacked'

interface EditorShellProps {
  /** What kind of edit this is — "Edit catalog", "Edit collection". Read out
   *  ahead of the title; on screen the sign's own colour says it. */
  purpose: string
  /** The region this editor belongs to: its header is that region's sign,
   *  and its Save and headings take that region's colour. */
  tone: 'catalog' | 'collection'
  /** Stickers stating facts about the subject — its kind, where it stands —
   *  beside the title on the sign from `sm` up, and heading the body below. */
  badges?: ReactNode
  /** The subject's next step with Community — Publish…, Unpublish… — as the
   *  sign's one button, before the way out; below `sm` it joins the stickers
   *  heading the body. Absent where there is none to take. */
  step: SignStep | undefined
  title: string
  /** Above `lg` this is the × in this header and Escape. Below it, it's also
   *  the mobile Library button — there is no separate scroll-only path back to
   *  the rail here, because leaving this row *is* an exit: it deselects, which
   *  a dirty form has to be able to hold and ask about. */
  onRequestClose: () => void
  /** The open row's own actions, mirrored here below `lg`. Absent in a
   *  collection's nested catalog editor, and until the library lists a row
   *  that was just created. */
  onDuplicate?: () => void
  onDelete?: () => void
  footer: ReactNode
  /** What docks beside the form, which decides the content cap: the form
   *  keeps its own `--w-form` column and the extra width goes to that column —
   *  the catalog editor's results (`--w-editor-results`) or the collection
   *  editor's preview (`--w-editor-preview`). */
  docked: 'results' | 'preview'
  children: ReactNode
}

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
  purpose,
  tone,
  badges,
  step,
  title,
  onRequestClose,
  onDuplicate,
  onDelete,
  footer,
  docked,
  children,
}: EditorShellProps) {
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
      // A dialog or menu is in front of the editor and Escape belongs to it.
      // Radix closes its layers from a capture listener on `document` and
      // marks the event `defaultPrevented`, so by the time this listener runs
      // the dialog may already be gone from the DOM: without both checks,
      // dismissing a discard prompt would immediately ask again, and
      // dismissing a delete confirmation would also try to close the editor
      // behind it.
      if (event.defaultPrevented || document.querySelector('[role="dialog"]')) return
      onRequestClose()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [onRequestClose, stacked])

  return (
    <main className={`tone-${tone} flex min-w-0 flex-1 flex-col lg:h-full lg:min-h-0`}>
      <PaneSign as="header" className="shrink-0 gap-3">
        <div className="flex min-w-0 flex-1 items-center gap-3">
          {/* Where focus lands when the page scrolls here. A scroll moves the
              viewport and nothing else, so without this a keyboard or
              screen-reader user is left behind in the rail. */}
          <h1
            tabIndex={-1}
            data-landing
            className={`m-0 truncate outline-none ${SIGN_TITLE}`}
          >
            <span className="sr-only">{purpose}: </span>
            {title}
          </h1>
          {badges && <div className="hidden shrink-0 items-center gap-1.5 sm:flex">{badges}</div>}
        </div>

        <SignStepButton step={step} place="sign" />

        {/* The row's actions and the way out, below `lg` only. */}
        <div className="flex shrink-0 items-center gap-1 lg:hidden">
          {onDuplicate && (
            <GlyphButton
              label={`Duplicate ${title}`}
              icon={Copy}
              onClick={onDuplicate}
              variant="icon"
            />
          )}
          {onDelete && (
            <GlyphButton
              label={`Delete ${title}`}
              icon={Trash2}
              onClick={onDelete}
              destructive
              variant="icon"
            />
          )}
          {/* Same call as the desktop ×, not a scroll of its own — see the
              doc comment on `onRequestClose` above. A held discard prompt has
              to be able to stop this exactly as it stops that one. */}
          <SignLibraryButton onClick={onRequestClose} title="Close editor (back to the library)" className="ml-1" />
        </div>

        <button
          type="button"
          onClick={onRequestClose}
          aria-label="Close editor and go back to your home screen"
          title="Close editor (Esc)"
          className="tap hover:bg-sign-ink/15 ml-auto hidden h-10 w-10 shrink-0 place-items-center rounded-full transition-colors lg:grid"
        >
          <Icon icon={X} size={20} />
        </button>
      </PaneSign>

      {/* One inset on all three bands — `px-4` narrow, `px-6` from `lg`: the
          header, the form, and the footer are stacked and share an edge, so a
          padding that differs between them reads as a misalignment rather than
          as a rhythm. The narrow value is smaller because 24px of gutter on
          each side of a 375px screen is 13% of it.

          `--w-form` caps the form itself, not the pane. The pane is as wide as
          the window allows and a setting is a label over a control — past
          about 700px a form stops being something you read down, and every
          field in it starts looking stretched. */}
      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-5 lg:px-6">
        {/* The sign hides its stickers and its step below `sm`, so they head
            the body. */}
        {badges && (
          <div className="mb-4 flex flex-wrap items-center gap-1.5 sm:hidden">
            {badges}
            <SignStepButton step={step} place="body" />
          </div>
        )}
        <div
          className={`w-full ${docked === 'preview' ? 'max-w-[var(--w-editor-preview)]' : 'max-w-[var(--w-editor-results)]'}`}
        >
          {children}
        </div>
      </div>

      {/* Sticky below `lg`, where the page scrolls as one document and Save
          would otherwise sit at the far end of a form as long as the catalog
          editor. It needs a ground of its own to sit over the form; above `lg`
          the flex column pins it and the band is transparent. */}
      <div className="border-line bg-ground sticky bottom-0 z-20 flex shrink-0 flex-wrap items-center justify-end gap-2 border-t px-4 py-4 lg:static lg:bg-transparent lg:px-6">
        {footer}
      </div>
    </main>
  )
}
