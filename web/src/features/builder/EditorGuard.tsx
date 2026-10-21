import { createContext, use, useCallback, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { ConfirmDialog } from '@/components/ConfirmDialog'

/**
 * Unsaved work in the pane's editor, and the single gate everything that would
 * discard it has to pass through.
 *
 * Lives above both the header and Configure because the exits are spread
 * across both: the editor's own × and Escape, selecting a different row in the
 * Library rail, and every navigation, which `LeaveGuard` asks through it — a
 * profile switch from the header among them. One gate rather than a
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
  /** `dirty` as of the last `setDirty`, before React renders it: what a
   *  navigation started in the same event as a `setDirty(false)` reads. */
  isDirty: () => boolean
  /**
   * Run something that would replace or discard the open editor.
   *
   * Runs immediately when there's nothing to lose, and otherwise holds it until
   * the confirmation is answered; `onCancel` runs if the answer is to stay.
   * Callers don't branch on `dirty` themselves — that's the whole point of
   * routing through here.
   */
  guard: (run: () => void, onCancel?: () => void) => void
  /** Set while a guarded action waits on the user. */
  blocked: boolean
  /** Discard and go: runs whatever `guard` was holding. */
  proceed: () => void
  /** Stay: drops the held action. */
  cancel: () => void
}

const EditorGuardContext = createContext<EditorGuardValue | null>(null)

export function EditorGuardProvider({ children }: { children: ReactNode }) {
  const [dirty, setDirtyState] = useState(false)
  const dirtyNow = useRef(false)
  // Held as an object wrapping the thunks: `useState` treats a bare function as
  // a lazy initialiser and would call it instead of storing it.
  const [pending, setPending] = useState<{ run: () => void; onCancel?: () => void } | null>(null)

  const setDirty = useCallback((next: boolean) => {
    dirtyNow.current = next
    setDirtyState(next)
  }, [])

  const isDirty = useCallback(() => dirtyNow.current, [])

  const guard = useCallback((run: () => void, onCancel?: () => void) => {
    if (dirtyNow.current) {
      setPending({ run, onCancel })
      return
    }
    run()
  }, [])

  // Read from the closure, not from a state updater: React may call an updater
  // twice in development, and running the held action twice would open two
  // editors or fire two navigations.
  const proceed = useCallback(() => {
    const run = pending?.run
    setPending(null)
    run?.()
  }, [pending])

  const cancel = useCallback(() => {
    const onCancel = pending?.onCancel
    setPending(null)
    onCancel?.()
  }, [pending])

  const value = useMemo<EditorGuardValue>(
    () => ({
      dirty,
      setDirty,
      isDirty,
      guard,
      blocked: pending !== null,
      proceed,
      cancel,
    }),
    [dirty, setDirty, isDirty, guard, pending, proceed, cancel],
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
 * own; the pane asks the same words through `Configure`'s prompt.
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
