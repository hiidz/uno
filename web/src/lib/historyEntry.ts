/**
 * Browser history entries that stand for something open on screen without a
 * path of its own — a Home folder page (`unoFolder`) — so the browser's own
 * Back closes it the way its in-app way out does.
 *
 * Each entry copies `history.state` before adding its flag, so React Router's
 * own keys ride along and it reads the entry as the location it already shows.
 */

/** Whether the history state carries `flag`. */
export function hasFlag(state: unknown, flag: string): boolean {
  return typeof state === 'object' && state !== null && (state as Record<string, unknown>)[flag] === true
}

/**
 * Whether a `popstate` left the entry its listener pushed: one was pushed, and
 * the entry now current no longer carries the flag. Stepping back off an entry
 * pushed on top of it, such as an editor's path, lands on this one, which
 * still carries it.
 */
export function leftEntry(pushed: boolean, state: unknown, flag: string): boolean {
  if (!pushed) return false
  return !hasFlag(state, flag)
}

/**
 * Pushes an entry carrying `flag` over the current one. False when the push
 * throws, as it can in a sandboxed frame.
 */
export function pushEntry(flag: string): boolean {
  const next: Record<string, unknown> = { ...(window.history.state as object | null), [flag]: true }
  try {
    window.history.pushState(next, '')
    return true
  } catch {
    return false
  }
}

/** Steps back off the entry carrying `flag`, if it is the current one. */
export function consumeEntry(flag: string) {
  if (hasFlag(window.history.state, flag)) window.history.back()
}
