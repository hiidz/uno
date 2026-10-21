import { useState, type MouseEvent } from 'react'

/** Where focus goes back to for `element`: itself, or the trigger of the
 *  menu it sits in, since a menu entry is gone once the menu closes. Radix
 *  labels a menu by its trigger. */
export function openerOf(element: Element | null): HTMLElement | null {
  const menu = element?.closest('[role="menu"]')
  const opener = menu ? document.getElementById(menu.getAttribute('aria-labelledby') ?? '') : element
  return opener instanceof HTMLElement && opener !== document.body ? opener : null
}

/** Whether focus has gone nowhere: nothing else has taken it since what held
 *  it went away. */
export function focusIsLost(): boolean {
  return document.activeElement === null || document.activeElement === document.body
}

/**
 * Puts focus back on `opener`, or on `fallback` when the opener is gone, as a
 * deleted row's is. Only while focus is lost: whatever took focus meanwhile
 * (a new editor's name field, a page's heading) keeps it.
 */
export function returnFocusTo(opener: HTMLElement | null, fallback?: HTMLElement | null): void {
  if (!focusIsLost()) return
  if (!opener?.isConnected) {
    fallback?.focus({ preventScroll: true })
    return
  }
  opener.focus({ preventScroll: true })
  if (fallback) followIfGone(opener, fallback)
}

/**
 * Moves focus on to `fallback` if `opener` leaves the page while it still
 * holds focus, within `GONE_FRAMES` frames: a deleted row's Delete button
 * goes when the library refetches, a moment after its dialog has closed.
 */
function followIfGone(opener: HTMLElement, fallback: HTMLElement): void {
  let frames = 0
  const watch = () => {
    if (focusIsLost() && !opener.isConnected) fallback.focus({ preventScroll: true })
    else if (document.activeElement === opener && ++frames < GONE_FRAMES) requestAnimationFrame(watch)
  }
  requestAnimationFrame(watch)
}

/** How many frames `followIfGone` watches, about two seconds. */
const GONE_FRAMES = 120

/** What sits outside the page's flow and closes on its own: a dialog or a
 *  menu, but not the editor's layer, which is a dialog only below `lg`. */
const OVERLAY = '[role="dialog"]:not([data-editor-layer]), [role="alertdialog"], [role="menu"]'

/**
 * `returnFocusTo`, once a dialog or menu closing at the same time has let go
 * of focus: while focus is inside one, this waits up to `CLOSE_FRAMES` frames
 * for it to close.
 */
export function returnFocusWhenFree(opener: HTMLElement | null, fallback?: HTMLElement | null): void {
  let frames = 0
  const attempt = () => {
    if (focusIsLost()) returnFocusTo(opener, fallback)
    else if (document.activeElement?.closest(OVERLAY) && ++frames < CLOSE_FRAMES) requestAnimationFrame(attempt)
  }
  attempt()
}

/** How many frames `returnFocusWhenFree` waits, about half a second. */
const CLOSE_FRAMES = 30

/** What focus goes back to on a dialog's close: its opener, else the region
 *  the opener sat in (`data-landing`), for an opener the dialog's action
 *  took away. */
interface Return {
  opener: HTMLElement | null
  region: HTMLElement | null
}

function returnFor(element: Element | null): Return {
  const opener = openerOf(element)
  return { opener, region: opener?.closest<HTMLElement>('[data-landing]') ?? null }
}

/**
 * A Radix dialog's `onCloseAutoFocus` for a dialog opened without a
 * `Dialog.Trigger`, which Radix would hand focus back to: the opener is
 * whatever held focus as `open` turned true, read during that render, before
 * the dialog takes focus.
 */
export function useReturnFocus(open: boolean): (event: Event) => void {
  const [target, setTarget] = useState<Return>(() => returnFor(open ? document.activeElement : null))
  const [wasOpen, setWasOpen] = useState(open)
  if (open !== wasOpen) {
    setWasOpen(open)
    if (open) setTarget(returnFor(document.activeElement))
  }
  return (event) => {
    event.preventDefault()
    returnFocusTo(target.opener, target.region)
  }
}

/**
 * Keeps focus on a row's move arrow after its row moves: React moves the row
 * by taking it out of the page and putting it back, which drops focus, and an
 * arrow that reaches the end of its list turns disabled, which drops it too.
 * The move can land a few frames after the press, so this watches for up to
 * `MOVE_FRAMES` frames while the arrow holds focus; once focus drops it goes
 * back to the arrow, or to the arrow beside it while this one is disabled.
 */
export function keepFocusOnMove(event: MouseEvent<HTMLButtonElement>): void {
  const arrow = event.currentTarget
  let frames = 0
  const watch = () => {
    if (!arrow.isConnected) return
    if (focusIsLost()) {
      const beside = [...(arrow.parentElement?.querySelectorAll('button') ?? [])].find((button) => button !== arrow)
      const target = arrow.disabled ? beside : arrow
      if (target && !target.disabled) target.focus({ preventScroll: true })
      return
    }
    if (document.activeElement === arrow && ++frames < MOVE_FRAMES) requestAnimationFrame(watch)
  }
  requestAnimationFrame(watch)
}

/** How many frames `keepFocusOnMove` watches, about half a second. */
const MOVE_FRAMES = 30
