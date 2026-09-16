import type { ReactNode } from 'react'
import { Dialog } from 'radix-ui'

/**
 * Scrim + panel, for the few things that genuinely interrupt: the short,
 * blocking question — naming a catalog before it exists — where the whole
 * point is that nothing else can be done until it's answered. The catalog and
 * collection editors fill the builder's right pane instead of this.
 *
 * Built on Radix `Dialog` for a real focus trap and return-focus-on-close —
 * the hand-rolled scrim this replaced had Escape and click-outside but no Tab
 * cycling. `labelledBy` is kept for callers that already `id` their own
 * heading; new callers can pass a `Dialog.Title` as the first child instead.
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
  return (
    <Dialog.Root open={open} onOpenChange={(next) => !next && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 grid place-items-center overscroll-contain bg-[var(--uno-scrim)] p-4 sm:p-8">
          <Dialog.Content
            aria-labelledby={labelledBy}
            style={{ width: `min(${width}, 100%)` }}
            className="bg-raised border-line-hi flex max-h-full flex-col overflow-hidden border outline-none"
          >
            {children}
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
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
