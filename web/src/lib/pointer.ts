/** Whether the main pointer is a precise one (a mouse, a trackpad). False
 *  where `matchMedia` doesn't exist (jsdom) and on touch, where focusing a
 *  field opens the on-screen keyboard. Read per call, like
 *  `prefersReducedMotion`. */
export function hasFinePointer(): boolean {
  return typeof window.matchMedia === 'function' && window.matchMedia('(pointer: fine)').matches
}
