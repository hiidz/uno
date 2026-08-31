import { useEffect } from 'react'
import type { ReactNode } from 'react'

/**
 * Scrim + panel, for the few things that genuinely interrupt.
 *
 * The builders used to live in one; they fill the right pane now. What's left
 * is the short, blocking question — naming a catalog before it exists — where
 * the whole point is that nothing else can be done until it's answered.
 */
export function Modal({
  open,
  onClose,
  labelledBy,
  width = '520px',
  children,
}: {
  open: boolean
  onClose: () => void
  labelledBy: string
  width?: string
  children: ReactNode
}) {
  useEffect(() => {
    if (!open) return
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKeyDown)
    // Nested scrolling inside a full-height panel fights with the page
    // scrolling behind it; lock the body while one is open. Safari ignores this
    // for its rubber-band, which is what `overscroll-contain` on the scrim and
    // the body covers.
    const previous = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onKeyDown)
      document.body.style.overflow = previous
    }
  }, [open, onClose])

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center overscroll-contain bg-[rgba(4,5,7,0.72)] p-4 sm:p-8"
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={labelledBy}
        style={{ width: `min(${width}, 100%)` }}
        className="bg-ground border-line-hi flex max-h-full flex-col overflow-hidden border"
      >
        {children}
      </div>
    </div>
  )
}

export function ModalHeader({ children }: { children: ReactNode }) {
  return (
    <div className="bg-raised border-line flex shrink-0 items-center gap-3 border-b px-5 py-3.5">
      {children}
    </div>
  )
}

export function ModalBody({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 py-4">{children}</div>
  )
}

export function ModalFooter({ children }: { children: ReactNode }) {
  return (
    <div className="border-line flex shrink-0 items-center justify-end gap-2 border-t px-5 py-3.5">
      {children}
    </div>
  )
}
