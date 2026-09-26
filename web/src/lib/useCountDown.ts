import { useEffect, useState } from 'react'
import { prefersReducedMotion } from './motion'

/**
 * `value`, except that when it drops to zero in the same render that `cue`
 * arrives — a push's success outcome — it counts down to zero over `duration`
 * ms instead of jumping: the pending count ringing down as a push lands. Any
 * other change shows at once, a drop to zero with no new `cue` included, and
 * so does this one under `prefers-reduced-motion`.
 */
export function useCountDown(value: number, cue: object | null, duration = 420): number {
  const [seen, setSeen] = useState({ value, cue })
  // The count being rung down, while one is, and the number it has reached.
  const [from, setFrom] = useState<number | null>(null)
  const [shown, setShown] = useState<number | null>(null)

  // Adjusted while rendering, not in an effect, so the render that sees the
  // drop already holds the old number rather than painting a frame of zero.
  if (value !== seen.value || cue !== seen.cue) {
    const rings =
      value === 0 && seen.value > 0 && cue !== null && cue !== seen.cue && !prefersReducedMotion()
    setSeen({ value, cue })
    setFrom(rings ? seen.value : null)
    setShown(null)
  }

  useEffect(() => {
    if (from === null) return
    let start: number | undefined
    let frame = requestAnimationFrame(function tick(now) {
      start ??= now
      const t = Math.min(1, (now - start) / duration)
      if (t < 1) {
        setShown(Math.round(from * (1 - t) ** 3))
        frame = requestAnimationFrame(tick)
      } else {
        setFrom(null)
        setShown(null)
      }
    })
    return () => cancelAnimationFrame(frame)
  }, [from, duration])

  if (value !== 0 || from === null) return value
  return shown ?? from
}
