import { createContext, use, useCallback, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { ConfirmDialog } from '@/components/ConfirmDialog'

/**
 * Unsaved work in the pane's editor, and the single gate everything that would
 * discard it has to pass through.
 *
 * Lives above both the header and the workspace because the exits are spread
 * across both: the editor's own × and Escape, selecting a different row in the
 * Library rail, and switching profile from the header. One gate rather than a
 * check per exit — a new exit added later inherits the guard by construction
 * instead of by someone remembering.
 *
 * The pane is the only thing that reports dirtiness. Home's own unpushed
 * changes are a separate question with a separate warning: they survive
 * closing an editor, and they aren't lost by opening one.
 */
interface EditorGuardValue {
  /** The open editor has changes that aren't saved. */
  dirty: boolean
  setDirty: (dirty: boolean) => void
  /**
   * Run something that would replace or discard the open editor.
   *
   * Runs immediately when there's nothing to lose, and otherwise holds it until
   * the confirmation is answered. Callers don't branch on `dirty` themselves —
   * that's the whole point of routing through here.
   */
  guard: (run: () => void) => void
  /** Set while a guarded action waits on the user. */
  blocked: boolean
  /** Discard and go: runs whatever `guard` was holding. */
  proceed: () => void
  /** Stay: drops the held action. */
  cancel: () => void
}

const EditorGuardContext = createContext<EditorGuardValue | null>(null)

export function EditorGuardProvider({ children }: { children: ReactNode }) {
  const [dirty, setDirty] = useState(false)
  // Held as an object wrapping the thunk: `useState` treats a bare function as
  // a lazy initialiser and would call it instead of storing it.
  const [pending, setPending] = useState<{ run: () => void } | null>(null)

  const guard = useCallback(
    (run: () => void) => {
      if (dirty) {
        setPending({ run })
        return
      }
      run()
    },
    [dirty],
  )

  // Read from the closure, not from a state updater: React may call an updater
  // twice in development, and running the held action twice would open two
  // editors or fire two navigations.
  const proceed = useCallback(() => {
    const run = pending?.run
    setPending(null)
    run?.()
  }, [pending])

  const value = useMemo<EditorGuardValue>(
    () => ({
      dirty,
      setDirty,
      guard,
      blocked: pending !== null,
      proceed,
      cancel: () => setPending(null),
    }),
    [dirty, guard, pending, proceed],
  )

  return <EditorGuardContext.Provider value={value}>{children}</EditorGuardContext.Provider>
}

export function useEditorGuard(): EditorGuardValue {
  const value = use(EditorGuardContext)
  if (!value) throw new Error('useEditorGuard must be used inside an EditorGuardProvider')
  return value
}

/**
 * The question a held exit asks: "Discard unsaved changes?", naming what is
 * being edited, with Keep editing and Discard. Draws nothing while no exit is
 * held. A collection's nested catalog editor asks it through a provider of its
 * own; the pane asks the same words through `Workspace`'s prompt.
 */
export function DiscardPrompt({ subject }: { subject: string }) {
  const { blocked, proceed, cancel } = useEditorGuard()
  return (
    <ConfirmDialog
      open={blocked}
      title="Discard unsaved changes?"
      body={
        <>
          Your changes to <strong className="text-ink">{subject}</strong> haven't been saved. Leaving
          discards them.
        </>
      }
      confirmLabel="Discard"
      cancelLabel="Keep editing"
      destructive
      onConfirm={proceed}
      onCancel={cancel}
    />
  )
}
