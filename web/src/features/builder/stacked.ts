import { useCallback, useLayoutEffect, useState, useSyncExternalStore, type RefObject } from 'react'
import { LG_MEDIA_QUERY } from '@/lib/breakpoints'
import { prefersReducedMotion } from '@/lib/motion'

/**
 * The one-column builder, and the scrolling that makes it usable.
 *
 * Above `lg` the rail and the pane are two columns of a fixed-height grid, each
 * scrolling on its own, and nothing in here applies. Below it there is a single
 * column and a single scrolling document: the rail on top, the Home pane
 * underneath, **both always mounted**. An open editor does not join that
 * document; it covers it as a layer (`EditorLayer`), and the page underneath
 * stays where it was.
 *
 * What moves the page is the pair of shortcuts between the two regions: the
 * rail's "Your home screen" down to Home, and Home's Library button back up.
 * Two things have to be true for those to work, and both live here: the sticky
 * app header's height has to be known (it is measured, not assumed), and the
 * scroll has to happen after the browser has laid out whatever it is scrolling
 * to.
 */

let lgQuery: MediaQueryList | null = null

/**
 * Resolved on first use rather than at import.
 *
 * A `matchMedia` call in module scope makes merely *importing* this module
 * depend on a browser API — one jsdom, the test environment, does not
 * implement. Missing, every query
 * answers as though this were a wide screen: the stacked layout is the one
 * with the extra behaviour, so not-stacked is the inert default.
 */
function media(query: string): MediaQueryList | null {
  if (typeof window.matchMedia !== 'function') return null
  return window.matchMedia(query)
}

function lg(): MediaQueryList | null {
  lgQuery ??= media(LG_MEDIA_QUERY)
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
type ScrollDestination = 'pane' | 'rail'

export interface ScrollRequest {
  to: ScrollDestination
  /** Two requests for the same destination are still two requests — pressing
   *  the same shortcut twice has to scroll twice. Identity is what the effect
   *  keys on, so this only has to differ. */
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
 * The `requestAnimationFrame` reads the destination in the next frame, after
 * layout, so a region whose height has just changed is scrolled to where it
 * now is.
 *
 * The rail is reached by scrolling to the top of the page rather than to the
 * rail itself: the tab row sits above the rail and outside the sticky header,
 * so lining the rail up under the header would leave the tabs just out of view.
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
      if (request.to === 'rail') window.scrollTo({ top: 0, behavior: scrollBehavior() })
      else element.scrollIntoView({ behavior: scrollBehavior(), block: 'start' })
      land(element)
    })
    return () => cancelAnimationFrame(frame)
  }, [stacked, request, paneRef, railRef])
}

/**
 * Move focus to whatever the region nominates with `data-landing`.
 *
 * A scroll moves the viewport and nothing else, so on its own it leaves a
 * keyboard or screen-reader user behind while the page travels somewhere they
 * were never told about; the editor layer lands the same way when it opens.
 * `preventScroll` because the scroll, if any, is already happening — letting
 * focus scroll too fights the smooth animation and lands short.
 */
export function land(region: HTMLElement) {
  const target = region.matches('[data-landing]')
    ? region
    : region.querySelector<HTMLElement>('[data-landing]')
  target?.focus({ preventScroll: true })
}

/** Smooth by default; instant for anyone who has asked for less motion. */
function scrollBehavior(): ScrollBehavior {
  return prefersReducedMotion() ? 'auto' : 'smooth'
}
