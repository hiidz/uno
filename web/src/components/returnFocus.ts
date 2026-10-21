import { useState } from 'react'

/** Where focus goes back to for `element`: itself, or the trigger of the
 *  menu it sits in, since a menu entry is gone once the menu closes. Radix
 *  labels a menu by its trigger. */
export function openerOf(element: Element | null): HTMLElement | null {
  const menu = element?.closest('[role="menu"]')
  const opener = menu ? document.getElementById(menu.getAttribute('aria-labelledby') ?? '') : element
  return opener instanceof HTMLElement && opener !== document.body ? opener : null
}

/**
 * Puts focus back on `opener`, or on `fallback` when the opener is gone, as a
 * deleted row's is. Only while focus is lost: whatever took focus meanwhile
 * (a new editor's title, a page's heading) keeps it.
 */
export function returnFocusTo(opener: HTMLElement | null, fallback?: HTMLElement | null): void {
  if (document.activeElement !== null && document.activeElement !== document.body) return
  const target = opener?.isConnected ? opener : fallback
  target?.focus({ preventScroll: true })
}

/**
 * A Radix dialog's `onCloseAutoFocus` for a dialog opened without a
 * `Dialog.Trigger`, which Radix would hand focus back to: the opener is
 * whatever held focus as `open` turned true, read during that render, before
 * the dialog takes focus.
 */
export function useReturnFocus(open: boolean): (event: Event) => void {
  const [opener, setOpener] = useState(() => (open ? openerOf(document.activeElement) : null))
  const [wasOpen, setWasOpen] = useState(open)
  if (open !== wasOpen) {
    setWasOpen(open)
    if (open) setOpener(openerOf(document.activeElement))
  }
  return (event) => {
    event.preventDefault()
    returnFocusTo(opener)
  }
}
