import { useEffect } from 'react'
import { Navigate, useLocation, useNavigate, type Location } from 'react-router-dom'
import type { ToastInput } from '@/components/toastQueue'

/** The navigation state that sends someone to a step that supplies what an
 *  address needs first, sign-in or a profile, and back to `location` once it
 *  is done; the shape `RequireAuth` sends to sign-in. */
export function returnState(location: Location): { from: Location } {
  return { from: location }
}

/** Where a step reached through `returnState` goes once done: the address it
 *  came from, or `fallback` when it came from none, or from sign-in or the
 *  picker themselves. */
export function returnPath(state: unknown, fallback: string): string {
  const from = (state as { from?: Location } | null)?.from
  if (!from || from.pathname === '/login' || from.pathname === '/profiles') return fallback
  return `${from.pathname}${from.search}${from.hash}`
}

/** To the profile picker, and back to this address once a profile is
 *  picked: no profile is picked, or the one picked isn't the account's. */
export function ToPicker() {
  const location = useLocation()
  return <Navigate to="/profiles" replace state={returnState(location)} />
}

/** To `to`, a screen that says `notice` as it opens: the address named a
 *  record that isn't there. */
export function ToParent({ to, notice }: { to: string; notice: string }) {
  return <Navigate to={to} replace state={{ notice }} />
}

/** Says the notice `ToParent` carried here, once: the entry's state is
 *  cleared as it is shown, so a reload doesn't say it again. */
export function useRedirectNotice(show: (toast: ToastInput) => void) {
  const location = useLocation()
  const navigate = useNavigate()
  const notice = (location.state as { notice?: unknown } | null)?.notice
  const text = typeof notice === 'string' ? notice : null
  useEffect(() => {
    if (text === null) return
    show({ text, tone: 'notice' })
    void navigate(location.pathname, { replace: true, state: null })
    // Once per notice: `show` and `navigate` are stable, and the path is the
    // one the notice arrived on.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [text])
}
