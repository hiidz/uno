import { useCallback, useLayoutEffect, useRef, useState } from 'react'

/** How far the window and a view's own column are scrolled. */
type ScrollPosition = Record<'window' | 'column', number>

/** The id of a Community row's open button, which focus returns to when its
 *  page closes. */
export function rowButtonID(publicationID: string): string {
  return `community-row-${publicationID}`
}

/** Puts focus back on the row whose page just closed, where the list left
 *  off, without scrolling to it. */
export function focusRow(publicationID: string) {
  document.getElementById(rowButtonID(publicationID))?.focus({ preventScroll: true })
}

/**
 * Where the list was scrolled when a page opened, put back once the list is
 * drawn again, then `then` runs. Below `lg` the window scrolls; from `lg`
 * the view's own column (`scrollRef`) does, so both are kept.
 */
export function useScrollMemory() {
  const scrollRef = useRef<HTMLDivElement>(null)
  const saved = useRef<ScrollPosition | null>(null)
  const after = useRef<() => void>(() => {})
  const [restoring, setRestoring] = useState(0)

  useLayoutEffect(
    function putBack() {
      if (!saved.current) return
      scrollTo(scrollRef.current, saved.current)
      after.current()
      saved.current = null
    },
    [restoring],
  )

  const remember = useCallback(function remember() {
    saved.current = scrollPosition(scrollRef.current)
    scrollTo(scrollRef.current, { window: 0, column: 0 })
  }, [])

  const restore = useCallback(function restore(then: () => void) {
    after.current = then
    setRestoring((n) => n + 1)
  }, [])

  return { scrollRef, remember, restore }
}

export function scrollPosition(column: HTMLElement | null): ScrollPosition {
  return { window: window.scrollY, column: column?.scrollTop ?? 0 }
}

export function scrollTo(column: HTMLElement | null, position: ScrollPosition) {
  window.scrollTo(0, position.window)
  if (column) column.scrollTop = position.column
}
