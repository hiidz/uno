import { useSyncExternalStore } from 'react'
import { ApiError } from '@/api/http'

/**
 * Whether a call has come back saying the signed-in account's own TMDB key
 * can't be used — it has none, or TMDB doesn't accept it — on a server where
 * each account brings one. The server answers that, and nothing else, with a
 * `422`. Once set it stays until a key is saved or the account signs out: the
 * key is what Nuvio's rows load with too, so the problem outlasts the one
 * call that found it (`KeyProblemBanner`).
 */
let problem = false
const listeners = new Set<() => void>()

function set(next: boolean) {
  if (next === problem) return
  problem = next
  for (const listener of listeners) listener()
}

/** Notes a key problem when `error` is one. The query client calls this for
 *  every failed query and mutation. */
export function noteKeyProblem(error: unknown) {
  if (error instanceof ApiError && error.status === 422) set(true)
}

/** Clears the problem: a key was saved, or the account signed out. */
export function clearKeyProblem() {
  set(false)
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export function useKeyProblem(): boolean {
  return useSyncExternalStore(subscribe, () => problem)
}
