import { useEffect, useRef, useState } from 'react'
import type { CSSProperties, ReactNode, RefObject } from 'react'
import { returnFocusTo } from '@/components/returnFocus'
import { land } from './stacked'

interface EditorLayerProps {
  /** Below `lg`: the layer covers the page instead of filling the pane. */
  stacked: boolean
  /** What is open, as the layer's accessible name. */
  label: string
  /** Which row is open; a new one is a new opener to return focus to. */
  openKey: string
  /** A row just made: focus lands on its title from `lg` too. */
  landOnOpen?: boolean
  /** Called once focus has landed for `landOnOpen`. */
  onLanded?: () => void
  /** Where focus goes on close when whatever opened the layer is gone. */
  fallbackFocus: RefObject<HTMLElement | null>
  children: ReactNode
}

/**
 * The pane's slot for an open editor or From Community view.
 *
 * From `lg` it is the pane column beside the rail, and nothing more. Below
 * `lg` the same element becomes a layer over the whole page — header
 * included — so the rail and the Home pane stay where they were underneath
 * and nothing in the page moves while it is open. It is the same element at
 * every width, so crossing `lg` (a tablet turning) keeps the editor and its
 * unsaved edits.
 *
 * Covering the page, it is a modal: everything else on the page, header
 * included, is inert and does not scroll, focus starts on the editor's title and returns to what opened it,
 * and the browser's Back closes it, as it steps back off the editor's path. It sits at `z-[35]`, over the
 * app header (`z-30`) and under menus (`z-40`) and dialogs (`z-50`).
 */
export function EditorLayer({
  stacked,
  label,
  openKey,
  landOnOpen = false,
  onLanded,
  fallbackFocus,
  children,
}: EditorLayerProps) {
  const ref = useRef<HTMLDivElement>(null)
  // Before the focus hand-off: its cleanup returns focus into a page that has
  // to have stopped being inert first.
  useInertOthers(stacked, ref)
  useScrollLock(stacked)
  useFocusHandoff(stacked, openKey, ref, fallbackFocus)
  useLandOnOpen(landOnOpen, ref, onLanded)

  return (
    <div
      ref={ref}
      {...layerRole(stacked, label)}
      style={LAYER_STYLE}
      className="layer-in flex min-w-0 flex-col max-lg:fixed max-lg:inset-0 max-lg:bottom-[var(--keyboard-h,0px)] max-lg:z-[35] max-lg:overscroll-contain max-lg:bg-ground max-lg:pr-[env(safe-area-inset-right)] max-lg:pl-[env(safe-area-inset-left)] lg:min-h-0"
    >
      {children}
    </div>
  )
}

/** Inside the layer there is no app header for a sticky band to clear. Above
 *  `lg` nothing in the pane is sticky, so the value is inert there. */
const LAYER_STYLE = { '--app-h': '0px' } as CSSProperties

/** A modal dialog while it covers the page. `data-editor-layer` is what
 *  `EditorShell`'s Escape tells apart from a dialog in front of the editor. */
function layerRole(stacked: boolean, label: string) {
  if (!stacked) return {}
  return { role: 'dialog', 'aria-modal': true, 'aria-label': label, 'data-editor-layer': '' }
}

/**
 * Shuts the rest of the page out while the layer covers it — the rail, Home,
 * the app header and tab row — as a modal dialog does: every sibling of the
 * layer and of each of its ancestors up to `<body>` goes `inert`. A dialog
 * opened over the layer later portals into `<body>` after this ran, so it
 * stays reachable. Only what this made inert is let go again.
 */
function useInertOthers(stacked: boolean, ref: RefObject<HTMLDivElement | null>) {
  useEffect(() => {
    if (!stacked || !ref.current) return
    return inertOthers(ref.current)
  }, [stacked, ref])
}

function inertOthers(layer: HTMLElement): () => void {
  const shut: Element[] = []
  for (let node = layer; node.parentElement && node !== document.body; node = node.parentElement) {
    for (const sibling of node.parentElement.children) {
      if (sibling === node || sibling.hasAttribute('inert')) continue
      sibling.setAttribute('inert', '')
      shut.push(sibling)
    }
  }
  return () => {
    for (const element of shut) element.removeAttribute('inert')
  }
}

/** The page behind keeps its scroll position by not scrolling at all. */
function useScrollLock(stacked: boolean) {
  useEffect(() => {
    if (!stacked) return
    const root = document.documentElement
    const previous = root.style.overflow
    root.style.overflow = 'hidden'
    return () => {
      root.style.overflow = previous
    }
  }, [stacked])
}

/**
 * Focus to the editor's title on open below `lg`. On close, at every width,
 * back to what opened the editor, or to `fallback` when that is gone, as a
 * deleted row is. The opener is read as each row opens, during that render,
 * before anything moves focus; one opened from inside the editor (Duplicate)
 * keeps the opener from outside it. Passive effects, so on close the page
 * behind has already stopped being inert.
 */
function useFocusHandoff(
  stacked: boolean,
  openKey: string,
  ref: RefObject<HTMLDivElement | null>,
  fallback: RefObject<HTMLElement | null>,
) {
  const [opener, setOpener] = useState(() => openerElement(null))
  const [openedKey, setOpenedKey] = useState(openKey)
  if (openKey !== openedKey) {
    setOpenedKey(openKey)
    const next = openerElement(ref.current)
    if (next) setOpener(next)
  }
  const latest = useRef(opener)
  useEffect(() => {
    latest.current = opener
  })

  useEffect(() => {
    if (stacked && ref.current) land(ref.current)
  }, [stacked, ref])

  useEffect(() => {
    const rail = fallback.current
    return () => returnFocusTo(latest.current, rail)
  }, [fallback])
}

/** Focus to the title of a row just made, from `lg` too, where a row opened
 *  from the rail leaves focus on the rail. */
function useLandOnOpen(landOnOpen: boolean, ref: RefObject<HTMLDivElement | null>, onLanded?: () => void) {
  useEffect(() => {
    if (!landOnOpen || !ref.current) return
    land(ref.current)
    onLanded?.()
  }, [landOnOpen, ref, onLanded])
}

/** What holds focus, unless nothing does or it is inside the layer itself or
 *  a dialog over the page, which close as the row opens. */
function openerElement(layer: HTMLElement | null): HTMLElement | null {
  const active = document.activeElement
  if (!(active instanceof HTMLElement) || active === document.body || layer?.contains(active)) return null
  if (active.closest('[role="dialog"]:not([data-editor-layer]), [role="alertdialog"]')) return null
  return active
}
