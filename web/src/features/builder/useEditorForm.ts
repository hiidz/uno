import { useEffect, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'

/**
 * The state both builders keep around their fields: the form itself, whether
 * its errors are on show yet, and the dirtiness the pane guards on.
 *
 * `baseline` is the form as it was seeded, and a **new identity re-seeds it** —
 * so it has to be stable at the call site. A form object built fresh each
 * render would clear the fields between keystrokes.
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
  showErrors: boolean
  revealErrors: () => void
  /** Reveal what's wrong, or save. */
  submit: (errorCount: number, save: (state: T) => void) => void
} {
  const [state, setState] = useState<T>(baseline)
  const [showErrors, setShowErrors] = useState(false)

  useEffect(() => {
    setState(baseline)
    setShowErrors(false)
  }, [baseline])

  const dirty = !isSame(baseline, state)
  useEffect(() => onDirtyChange(dirty), [dirty, onDirtyChange])

  return {
    state,
    setState,
    showErrors,
    revealErrors: () => setShowErrors(true),
    submit(errorCount, save) {
      setShowErrors(true)
      if (errorCount > 0) return
      save(state)
    },
  }
}
