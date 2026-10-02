/**
 * Browser history entries that stand for something open on screen — a Home
 * folder page (`unoFolder`), the editor layer below `lg` (`unoEditor`) — so the
 * browser's own Back closes it the way its in-app way out does.
 *
 * Each entry copies `history.state` before adding its flag, so React Router's
 * own state rides along and a Forward onto the entry still hands the builder
 * its profile.
 */

/** Whether the history state carries `flag`. */
export function hasFlag(state: unknown, flag: string): boolean {
  return typeof state === 'object' && state !== null && (state as Record<string, unknown>)[flag] === true
}

/**
 * Whether a `popstate` left the entry its listener pushed: one was pushed, and
 * the entry now current no longer carries the flag. An entry pushed on top of
 * it by something else copies the flag, so stepping back off that one is not
 * a step off this one.
 */
export function leftEntry(pushed: boolean, state: unknown, flag: string): boolean {
  if (!pushed) return false
  return !hasFlag(state, flag)
}

/**
 * Pushes an entry carrying `flag` over the current one. `clear` names flags
 * the new entry drops, so a step back keyed on them never lands on this entry
 * by mistake. False when the push throws, as it can in a sandboxed frame.
 */
export function pushEntry(flag: string, clear: string[] = []): boolean {
  const next: Record<string, unknown> = { ...(window.history.state as object | null), [flag]: true }
  for (const key of clear) delete next[key]
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
