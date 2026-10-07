import { useEffect, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'

/**
 * The state both builders keep around their fields: the form itself, whether
 * its errors are on show yet, and the dirtiness the pane guards on.
 *
 * `baseline` is the form as it was seeded. It is read once, on mount: a
 * different subject is a different mounted editor (callers key it), never a new
 * baseline under the same one.
 *
 * **Dirtiness is reported, not handled.** Every way out of an editor originates
 * outside it — the × in the shell, Escape, selecting another row in the rail,
 * switching profile — so the pane owns the confirmation and an editor only has
 * to say whether there is anything to lose.
 *
 * Errors stay hidden until something is submitted, which is why revealing them
 * is its own callback: the catalog builder's Preview button also has to bring
 * the highlighting on, or its note explaining why it won't run would point at
 * highlighting that isn't there yet.
 */
export function useEditorForm<T>(
  baseline: T,
  isSame: (a: T, b: T) => boolean,
  onDirtyChange: (dirty: boolean) => void,
): {
  state: T
  setState: Dispatch<SetStateAction<T>>
  /** The form differs from `baseline` — what `onDirtyChange` reports. */
  dirty: boolean
  showErrors: boolean
  revealErrors: () => void
  /** Reveal what's wrong, or save. */
  submit: (errorCount: number, save: (state: T) => void) => void
} {
  const [state, setState] = useState<T>(baseline)
  const [showErrors, setShowErrors] = useState(false)

  const dirty = !isSame(baseline, state)
  useEffect(() => onDirtyChange(dirty), [dirty, onDirtyChange])

  return {
    state,
    setState,
    dirty,
    showErrors,
    revealErrors: () => setShowErrors(true),
    submit(errorCount, save) {
      setShowErrors(true)
      if (errorCount > 0) return
      save(state)
    },
  }
}
