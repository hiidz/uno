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
    <main className="flex min-w-0 flex-col lg:h-full lg:min-h-0">
      <header className="border-line flex shrink-0 items-center gap-3 border-b px-6 py-4">
        <TypeBar kind={kind} owned={owned} className="min-h-[30px]" />
        <div className="flex min-w-0 flex-col gap-[3px]">
          <span className="type-eyebrow">{eyebrow}</span>
          <h1 className="type-display m-0 truncate text-[17px]">{title}</h1>
        </div>
        <button
          type="button"
          onClick={onRequestClose}
          aria-label="Close editor and go back to your home screen"
          title="Close editor (Esc)"
          className="text-dim hover:text-ink hover:border-dim ml-auto grid h-7 w-7 shrink-0 place-items-center rounded-[2px] border border-transparent text-[15px] leading-none transition-colors"
        >
          ×
        </button>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">{children}</div>

      <div className="border-line flex shrink-0 flex-wrap items-center justify-end gap-2 border-t px-6 py-3.5">
        {footer}
      </div>
    </main>
  )
}
