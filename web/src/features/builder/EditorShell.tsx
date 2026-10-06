import { useEffect } from 'react'
import type { ReactNode } from 'react'
import { Copy, Trash2, X } from 'lucide-react'
import { GlyphButton } from '@/components/GlyphButton'
import { Icon } from '@/components/Icon'
import { PaneSign, SIGN_TITLE } from '@/components/PaneSign'

/** A dialog or menu in front of the editor — any `role="dialog"` but the
 *  editor's own layer below `lg` (`EditorLayer`). */
const DIALOG_IN_FRONT = '[role="dialog"]:not([data-editor-layer])'

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
  title: string
  /** The × in this header and Escape. */
  onRequestClose: () => void
  /** The open row's own actions, mirrored here below `lg`. Absent in a
   *  collection's nested catalog editor, and until the library lists a row
   *  that was just created. */
  onDuplicate?: () => void
  onDelete?: () => void
  footer: ReactNode
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
 * call as × rather than closing directly.
 *
 * **Below `lg` the header carries the library row's own duplicate and
 * delete.** Those actions live on the row itself above `lg`, where the rail is
 * beside the pane; below it the editor is a layer over the rail
 * (`EditorLayer`), and this header is the one place they can be.
 *
 * The body scrolls between a header and a footer that stay put, at every
 * width: the pane column holds the editor from `lg`, and the layer below it.
 */
export function EditorShell({
  purpose,
  tone,
  badges,
  title,
  onRequestClose,
  onDuplicate,
  onDelete,
  footer,
  children,
}: EditorShellProps) {
  // Escape closes, matching the convention every modal dialog sets — the
  // muscle memory is already there and there's no reason to take it away.
  // Routed through `onRequestClose` so a dirty form still confirms.
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== 'Escape') return
      // A dialog or menu is in front of the editor and Escape belongs to it.
      // Radix closes its layers from a capture listener on `document` and
      // marks the event `defaultPrevented`, so by the time this listener runs
      // the dialog may already be gone from the DOM: without both checks,
      // dismissing a discard prompt would immediately ask again, and
      // dismissing a delete confirmation would also try to close the editor
      // behind it. The editor's own layer below `lg` is a dialog too, and is
      // not one in front of it.
      if (event.defaultPrevented || document.querySelector(DIALOG_IN_FRONT)) return
      // Handled: a Home folder page open under the layer listens on `window`,
      // after this, and leaves an Escape that closed the editor alone.
      event.preventDefault()
      onRequestClose()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [onRequestClose])

  return (
    <main className={`tone-${tone} flex h-full min-h-0 min-w-0 flex-1 flex-col`}>
      <PaneSign as="header" className="shrink-0 gap-3">
        <div className="flex min-w-0 flex-1 items-center gap-3">
          {/* Where focus lands when the editor's layer opens below `lg`. */}
          <h1
            tabIndex={-1}
            data-landing
            className={`m-0 outline-none ${SIGN_TITLE}`}
          >
            <span className="sr-only">{purpose}: </span>
            {title}
          </h1>
          {badges && <div className="hidden shrink-0 items-center gap-1.5 sm:flex">{badges}</div>}
        </div>

        {/* The row's actions, below `lg` only. */}
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
        </div>

        <button
          type="button"
          onClick={onRequestClose}
          aria-label="Close editor"
          title="Close (Esc)"
          className="tap hover:bg-sign-ink/15 ml-auto grid h-10 w-10 shrink-0 place-items-center rounded-full transition-colors"
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
        {/* The sign hides its stickers below `sm`, so they head the body. */}
        {badges && <div className="mb-4 flex flex-wrap items-center gap-1.5 sm:hidden">{badges}</div>}
        {children}
      </div>

      {/* The flex column pins it under the scrolling body at every width. */}
      <div className="border-line flex shrink-0 flex-wrap items-center justify-end gap-2 border-t px-4 py-4 lg:px-6">
        {footer}
      </div>
    </main>
  )
}
