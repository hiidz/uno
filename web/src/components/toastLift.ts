import { useCallback } from 'react'

/** Each mounted bar the toasts must clear, by its height in px. */
const lifts = new Map<object, number>()

/** The lift the toasts take: the tallest bar's, or 0 with none. */
export function toastLift(heights: Iterable<number>): number {
  return Math.max(0, ...heights)
}

function apply() {
  document.documentElement.style.setProperty('--toast-lift', `${toastLift(lifts.values())}px`)
}

/**
 * A ref for a bar along the screen's foot (the jump button, an editor's save
 * bar): while the bar is mounted, `--toast-lift` holds its height plus `gap`,
 * so the toasts sit above it. A bar that isn't drawn (`display: none`) lifts
 * nothing. With several mounted, the tallest wins.
 */
export function useToastLift(gap = 0) {
  return useCallback(
    (element: HTMLElement | null) => {
      if (!element) return
      const key = {}
      const measure = () => {
        lifts.set(key, element.offsetHeight > 0 ? element.offsetHeight + gap : 0)
        apply()
      }
      const observer = new ResizeObserver(measure)
      observer.observe(element)
      measure()
      return () => {
        observer.disconnect()
        lifts.delete(key)
        apply()
      }
    },
    [gap],
  )
}
