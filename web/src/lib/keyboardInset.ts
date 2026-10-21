/**
 * Keeps `--keyboard-h` on the root at how much of the window an on-screen
 * keyboard covers, so a full-screen layer or dialog can end above it and its
 * buttons stay reachable. iOS Safari lays the keyboard over the page without
 * resizing it; only the visual viewport shrinks. While the page is pinch-zoomed
 * the space below the visual viewport is the zoom, not a keyboard, so it
 * counts as none.
 */
export function watchKeyboardInset() {
  const viewport = window.visualViewport
  if (!viewport) return
  const update = () => {
    const covered = viewport.scale > 1.01 ? 0 : window.innerHeight - viewport.height - viewport.offsetTop
    document.documentElement.style.setProperty('--keyboard-h', `${Math.max(0, Math.round(covered))}px`)
  }
  viewport.addEventListener('resize', update)
  viewport.addEventListener('scroll', update)
  update()
}
