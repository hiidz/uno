import { useEffect, useRef, useState } from 'react'
import { consumeEntry, leftEntry, pushEntry } from '@/lib/historyEntry'

/** The flag on the editor layer's history entry. */
const EDITOR_ENTRY = 'unoEditor'

/** The flag on a Home folder page's entry, which the layer's entry drops so
 *  stepping back off a folder page never steps off the layer instead. */
const FOLDER_ENTRY = 'unoFolder'

/**
 * The browser's Back as a way out of the editor layer below `lg`.
 *
 * While `open`, the layer holds one history entry. Back pops it and asks to
 * close through `requestClose`, the same guarded close as × — so a dirty form
 * asks first. Keep editing leaves the layer open with its entry gone, and the
 * entry goes back on, so the next Back asks again. Every other way out
 * consumes the entry with one `history.back()`, so no later Back lands on an
 * entry nobody listens for.
 *
 * The step back on unmount waits a tick and is called off by a remount, which
 * is what StrictMode's mount, unmount, mount does in development: stepping
 * back there would pop the entry the second mount still holds.
 */
export function useLayerHistory(open: boolean, requestClose: () => void) {
  const pushed = useRef(false)
  const leaving = useRef<number | null>(null)
  const latestClose = useRef(requestClose)
  const [revision, setRevision] = useState(0)

  useEffect(() => {
    latestClose.current = requestClose
  })

  useEffect(() => {
    syncEntry(open, pushed)
  }, [open, revision])

  useEffect(() => {
    callOff(leaving)

    function onPopState(event: PopStateEvent) {
      if (!leftEntry(pushed.current, event.state, EDITOR_ENTRY)) return
      pushed.current = false
      setRevision((n) => n + 1)
      latestClose.current()
    }

    window.addEventListener('popstate', onPopState)
    return () => {
      window.removeEventListener('popstate', onPopState)
      leaving.current = window.setTimeout(() => leave(pushed), 0)
    }
  }, [])
}

type Flag = { current: boolean }

/** Puts the entry on while open, and takes it off once not. */
function syncEntry(open: boolean, pushed: Flag) {
  if (open === pushed.current) return
  if (open) {
    pushed.current = pushEntry(EDITOR_ENTRY, [FOLDER_ENTRY])
    return
  }
  leave(pushed)
}

function leave(pushed: Flag) {
  if (!pushed.current) return
  pushed.current = false
  consumeEntry(EDITOR_ENTRY)
}

function callOff(leaving: { current: number | null }) {
  if (leaving.current === null) return
  window.clearTimeout(leaving.current)
  leaving.current = null
}
