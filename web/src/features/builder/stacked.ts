import { useCallback, useLayoutEffect, useState, useSyncExternalStore, type RefObject } from 'react'

/**
 * The one-column builder, and the scrolling that makes it usable.
 *
 * Above `lg` the rail and the pane are two columns of a fixed-height grid, each
 * scrolling on its own, and nothing in here applies. Below it there is a single
 * column and a single scrolling document: the rail on top, the pane underneath,
 * **both always mounted**. Selecting a row doesn't hide anything — it scrolls
 * the page to the pane, which is why the pane being far down the document is no
 * longer the problem it was when the user had to find it themselves.
 *
 * Two things have to be true for that to work, and both live here: the sticky
 * app header's height has to be known (it is measured, not assumed), and the
 * scroll has to happen after the browser has laid out whatever it is scrolling
 * to.
 */

/** Tailwind's `lg`, as the query the class generates. */
const LG = '(min-width: 64rem)'

let lgQuery: MediaQueryList | null = null

/**
 * Resolved on first use rather than at import.
 *
 * The router imports this module's consumers eagerly, so a `matchMedia` call in
 * module scope makes *loading* the app depend on a browser API — which is how
 * the app's mount smoke test found it, running under jsdom. Missing, every
 * query answers as though this were a wide screen: the stacked layout is the
 * one with the extra behaviour, so not-stacked is the inert default.
 */
function media(query: string): MediaQueryList | null {
  if (typeof window.matchMedia !== 'function') return null
  return window.matchMedia(query)
}

function lg(): MediaQueryList | null {
  lgQuery ??= media(LG)
  return lgQuery
}

function subscribeToLG(onChange: () => void) {
  const query = lg()
  if (!query) return () => {}
  query.addEventListener('change', onChange)
  return () => query.removeEventListener('change', onChange)
}

/** True while the two regions are stacked into one scrolling document. */
export function useStackedLayout(): boolean {
  return useSyncExternalStore(subscribeToLG, () => !(lg()?.matches ?? true))
}

/**
 * Publish the sticky app header's height as `--app-h` on the document root.
 *
 * Every offset below `lg` is measured from the bottom of that header: what
 * `scroll-margin-top` has to clear, where the pane's own sticky header pins
 * itself, and how tall the pane has to be to be scrollable to the top. The
 * header is one row at a fixed height, but a constant would still be wrong in
 * normal use: the push banner mounts and unmounts underneath it while the page
 * is open, and its own height varies with the outcome it reports.
 *
 * Observe the **wrapper**, not the `<header>`: the banner is the wrapper's
 * second child, so measuring the header alone loses it exactly when it is
 * there.
 */
export function usePublishedHeaderHeight(ref: RefObject<HTMLElement | null>) {
  useLayoutEffect(() => {
    const element = ref.current
    if (!element) return

    const publish = () => {
      document.documentElement.style.setProperty('--app-h', `${element.offsetHeight}px`)
    }

    publish()
    const observer = new ResizeObserver(publish)
    observer.observe(element)
    return () => {
      observer.disconnect()
      // Back to the fallback in `index.css`, so a stale measurement can't
      // outlive the header it came from.
      document.documentElement.style.removeProperty('--app-h')
    }
  }, [ref])
}

/** Which region a scroll is asking for. */
export type ScrollDestination = 'pane' | 'rail'

export interface ScrollRequest {
  to: ScrollDestination
  /** Two requests for the same destination are still two requests — re-tapping
   *  the open row has to scroll again. Identity is what the effect keys on, so
   *  this only has to differ. */
  seq: number
}

export function useScrollRequests() {
  const [request, setRequest] = useState<ScrollRequest | null>(null)

  const requestScroll = useCallback((to: ScrollDestination) => {
    setRequest((previous) => ({ to, seq: (previous?.seq ?? 0) + 1 }))
  }, [])

  return { request, requestScroll }
}

/**
 * Perform a requested scroll, once the thing being scrolled to exists.
 *
 * **Requests are made after the state that motivates them has been committed,
 * never in the event handler that started it.** That is what keeps the
 * unsaved-changes guard honest: `guard` holds its callback until the
 * confirmation is answered, so a held action makes no request, a cancelled one
 * makes no request, and a discarded one makes exactly one — after the target
 * has actually changed. A scroll fired from the tap instead would move the page
 * out from under a dialog the user hasn't answered yet.
 *
 * The `requestAnimationFrame` is not a delay for its own sake. The editor
 * remounts on every target change (`key` in `Workspace`), so at the moment the
 * effect runs the pane still has the outgoing editor's height; reading the
 * destination in the next frame reads it after layout.
 */
export function useStackedScroll({
  stacked,
  request,
  paneRef,
  railRef,
}: {
  stacked: boolean
  request: ScrollRequest | null
  paneRef: RefObject<HTMLElement | null>
  railRef: RefObject<HTMLElement | null>
}) {
  useLayoutEffect(() => {
    if (!stacked || request === null) return

    const element = request.to === 'pane' ? paneRef.current : railRef.current
    if (!element) return

    const frame = requestAnimationFrame(() => {
      element.scrollIntoView({ behavior: scrollBehavior(), block: 'start' })
      land(element)
    })
    return () => cancelAnimationFrame(frame)
  }, [stacked, request, paneRef, railRef])
}

/**
 * Move focus to whatever the region nominates with `data-landing`.
 *
 * A scroll moves the viewport and nothing else, so on its own it leaves a
 * keyboard or screen-reader user behind in the rail while the page travels
 * somewhere they were never told about. `preventScroll` because the scroll is
 * already happening — letting focus scroll too fights the smooth animation and
 * lands short.
 */
function land(region: HTMLElement) {
  const target = region.matches('[data-landing]')
    ? region
    : region.querySelector<HTMLElement>('[data-landing]')
  target?.focus({ preventScroll: true })
}

/** Smooth by default; instant for anyone who has asked for less motion. Read
 *  per scroll rather than cached, so changing the OS setting takes effect
 *  without a reload. */
function scrollBehavior(): ScrollBehavior {
  return media('(prefers-reduced-motion: reduce)')?.matches ? 'auto' : 'smooth'
}
