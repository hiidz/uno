import type { ReactNode } from 'react'
import { Dialog } from 'radix-ui'

/**
 * Scrim + panel, for what opens over the builder without replacing the pane's
 * occupant: naming a catalog or collection before it exists, import and
 * export, and a collection's nested catalog editor. The catalog and collection
 * editors themselves fill the builder's right pane instead of this.
 *
 * Built on Radix `Dialog` for a real focus trap and return-focus-on-close.
 * `labelledBy` is the `id` of the caller's own heading.
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
            className="bg-raised border-line-hi flex max-h-full flex-col overflow-hidden rounded-[20px] border outline-none"
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
    <div className="border-line flex shrink-0 items-center gap-3 border-b px-6 py-4">
      {children}
    </div>
  )
}

export function ModalBody({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-6 py-5">{children}</div>
  )
}

/** The dialog's buttons. `tone` gives its primary button a region's colour
 *  when the dialog belongs to one — creating a catalog, creating a
 *  collection. */
export function ModalFooter({
  tone,
  children,
}: {
  tone?: 'catalog' | 'collection'
  children: ReactNode
}) {
  return (
    <div
      className={`border-line flex shrink-0 items-center justify-end gap-2 border-t px-6 py-4 ${
        tone ? `tone-${tone}` : ''
      }`}
    >
      {children}
    </div>
  )
}
