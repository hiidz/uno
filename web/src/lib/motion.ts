/** Whether the viewer has asked for less motion. False where `matchMedia`
 *  doesn't exist (jsdom, very old engines) — the state that animates. Read
 *  per call rather than cached, so changing the OS setting takes effect
 *  without a reload. */
export function prefersReducedMotion(): boolean {
  return (
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
}
