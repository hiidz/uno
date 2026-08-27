import { useEffect } from 'react'

/**
 * Warns before a reload or tab close while edits are pending.
 *
 * Pending home-screen changes live only in browser memory until Push, so a
 * reload discards them with no server-side trace. This is the only thing
 * standing between the user and silently losing work that way; the in-app
 * exits (the profile chip) are guarded separately, with a dialog that can
 * actually say how much is at stake.
 *
 * Browsers ignore custom text here and show their own wording — setting
 * `returnValue` is just how you opt in.
 */
export function useUnloadGuard(enabled: boolean): void {
  useEffect(() => {
    if (!enabled) return
    function onBeforeUnload(event: BeforeUnloadEvent) {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [enabled])
}
