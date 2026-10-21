import { useEffect, useRef, useState } from 'react'
import { useBlocker } from 'react-router-dom'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { useEditorGuard } from '@/features/builder/EditorGuard'
import { useHomeSelection } from '@/features/home/useHomeSelection'
import { pluralCount } from '@/lib/plural'
import { leaveStep, type LeaveStep } from '@/features/builder/paths'

/**
 * The one gate every navigation inside or out of the builder passes, the
 * browser's Back and Forward included: the builder's own controls only
 * navigate, and this asks what the move would discard (`leaveStep`).
 *
 * The editor's question comes first, through `EditorGuard` — the prompt
 * `Workspace` draws — then, when the move leaves the builder, Home's. Moving
 * between the builder's sections keeps Home's state, which lives above both.
 * Each answer to stay resets the blocker, so the next navigation is asked
 * afresh.
 */
export function LeaveGuard() {
  const home = useHomeSelection()
  const editor = useEditorGuard()
  const homeDirty = useRef(home.isDirty)
  // What the blocked move would discard, noted as it is blocked.
  const blockedStep = useRef<LeaveStep>('none')
  const [askingHome, setAskingHome] = useState(false)

  useEffect(() => {
    homeDirty.current = home.isDirty
  })

  const blocker = useBlocker(({ currentLocation, nextLocation }) => {
    // A session that ended goes to sign-in unasked: nothing left could be
    // saved or pushed without one.
    if (nextLocation.pathname === '/login') return false
    const step = leaveStep(currentLocation.pathname, nextLocation.pathname)
    blockedStep.current = step
    if (step === 'none') return false
    return editor.isDirty() || (step === 'builder' && homeDirty.current)
  })

  const blocked = blocker.state === 'blocked'
  const { guard, setDirty } = editor
  useEffect(() => {
    if (!blocked) return
    const leaving = blockedStep.current === 'builder'
    guard(
      () => {
        setDirty(false)
        if (leaving && homeDirty.current) {
          setAskingHome(true)
          return
        }
        blocker.proceed?.()
      },
      () => blocker.reset?.(),
    )
    // Once per block, so only on `blocked`: `blocker` is a new object on
    // every render, and `guard` and `setDirty` are stable.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [blocked])

  return (
    <ConfirmDialog
      open={askingHome}
      title="Discard unpushed changes?"
      body={
        <>
          {pluralCount(home.unsavedCount, 'change')} to your home screen{' '}
          {home.unsavedCount === 1 ? "hasn't" : "haven't"} been pushed yet. Leaving discards{' '}
          {home.unsavedCount === 1 ? 'it' : 'them'}.
        </>
      }
      confirmLabel="Discard and leave"
      cancelLabel="Stay here"
      destructive
      onConfirm={() => {
        setAskingHome(false)
        blocker.proceed?.()
      }}
      onCancel={() => {
        setAskingHome(false)
        blocker.reset?.()
      }}
    />
  )
}
