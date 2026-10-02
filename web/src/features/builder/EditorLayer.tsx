import { useEffect, useRef, useState } from 'react'
import type { CSSProperties, ReactNode, RefObject } from 'react'
import { land } from './stacked'
import { useLayerHistory } from './useLayerHistory'

interface EditorLayerProps {
  /** Below `lg`: the layer covers the page instead of filling the pane. */
  stacked: boolean
  /** What is open, as the layer's accessible name. */
  label: string
  /** The guarded close — the browser's Back asks through it, as × does. */
  onRequestClose: () => void
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
 * and the browser's Back asks to close it. It sits at `z-[35]`, over the
 * app header (`z-30`) and under menus (`z-40`) and dialogs (`z-50`).
 */
export function EditorLayer({ stacked, label, onRequestClose, fallbackFocus, children }: EditorLayerProps) {
  const ref = useRef<HTMLDivElement>(null)
  useLayerHistory(stacked, onRequestClose)
  // Before the focus hand-off: its cleanup returns focus into a page that has
  // to have stopped being inert first.
  useInertOthers(stacked, ref)
  useScrollLock(stacked)
  useFocusHandoff(stacked, ref, fallbackFocus)

  return (
    <div
      ref={ref}
      {...layerRole(stacked, label)}
      style={LAYER_STYLE}
      className="layer-in flex min-w-0 flex-col max-lg:fixed max-lg:inset-0 max-lg:z-[35] max-lg:overscroll-contain max-lg:bg-ground lg:min-h-0"
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
 * Focus to the editor's title on open, and back to what opened the layer on
 * close — or to `fallback` when that is gone, as a deleted row is. The opener
 * is read once, on the first render, before anything moves focus. Passive
 * effects, so on close the page behind has already stopped being inert.
 */
function useFocusHandoff(
  stacked: boolean,
  ref: RefObject<HTMLDivElement | null>,
  fallback: RefObject<HTMLElement | null>,
) {
  const [opener] = useState(() => document.activeElement)

  useEffect(() => {
    if (!stacked) return
    const rail = fallback.current
    if (ref.current) land(ref.current)
    return () => returnFocus(opener, rail)
  }, [stacked, ref, opener, fallback])
}

function returnFocus(opener: Element | null, fallback: HTMLElement | null) {
  const target = opener instanceof HTMLElement && opener.isConnected && opener !== document.body ? opener : fallback
  target?.focus({ preventScroll: true })
}
