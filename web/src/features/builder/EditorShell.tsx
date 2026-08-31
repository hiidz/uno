import { useEffect } from 'react'
import type { ReactNode } from 'react'
import { TypeBar, type BarKind } from '@/components/TypeBar'

/**
 * The frame around an editor filling the builder's right pane.
 *
 * Replaces the modal chrome these forms used to sit in. The pane holds one
 * occupant at a time — either the home screen or one editor — so there is no
 * tab strip and no way to express a second open form: the title says what is
 * being edited, and the × closes it.
 *
 * **Closing is a request, not an act.** `onRequestClose` may raise a discard
 * confirmation instead of closing, which is why Escape routes through the same
 * call rather than closing directly.
 */
export function EditorShell({
  eyebrow,
  title,
  kind,
  owned,
  onRequestClose,
  onBack,
  footer,
  children,
}: {
  /** What kind of edit this is — "Edit catalog", "New collection". The title
   *  below it is the subject's own name, which is what the eye lands on. */
  eyebrow: string
  title: string
  /** Carries the rail row's hue into the pane, so the thing you clicked and the
   *  thing in front of you are visibly the same thing. */
  kind: BarKind
  owned: boolean
  onRequestClose: () => void
  /** Show the rail instead. Below `lg` only, where the two regions take turns.
   *  **Not an exit**, so it does not go through `onRequestClose`: above `lg`
   *  the rail is on screen beside an open editor, and this is the same thing
   *  said in one column. The editor stays open and stays dirty. */
  onBack: () => void
  footer: ReactNode
  children: ReactNode
}) {
  // Escape closes, matching the modal these forms replaced — the muscle memory
  // is already there and there's no reason to take it away. Routed through
  // `onRequestClose` so a dirty form still confirms.
  useEffect(() => {
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
  }, [onRequestClose])

  return (
    <main className="flex min-w-0 flex-1 flex-col lg:h-full lg:min-h-0">
      <header className="border-line flex shrink-0 items-center gap-3 border-b px-4 py-4 lg:px-6">
        <button
          type="button"
          onClick={onBack}
          aria-label="Back to the library"
          title="Back to the library"
          className="tap text-dim hover:text-ink -ml-1 grid h-8 w-6 shrink-0 place-items-center text-[15px] leading-none transition-colors lg:hidden"
        >
          ‹
        </button>
        <TypeBar kind={kind} owned={owned} className="min-h-[30px]" />
        <div className="flex min-w-0 flex-col gap-1">
          <span className="type-eyebrow">{eyebrow}</span>
          <h1 className="type-display m-0 truncate text-[17px]">{title}</h1>
        </div>
        <button
          type="button"
          onClick={onRequestClose}
          aria-label="Close editor and go back to your home screen"
          title="Close editor (Esc)"
          className="tap text-dim hover:text-ink hover:border-dim ml-auto grid h-7 w-7 shrink-0 place-items-center rounded-[2px] border border-transparent text-[15px] leading-none transition-colors"
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
