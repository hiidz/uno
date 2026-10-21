import type { ReactNode } from 'react'
import { AlertDialog } from 'radix-ui'
import { Button } from './Button'
import { useReturnFocus } from './returnFocus'

/**
 * A confirmation the user has to answer before something irreversible happens.
 *
 * Not `window.confirm`: this app has to say *what* is at stake ("3 unpushed
 * changes", "removes it from Community"), and a native dialog gives no
 * room for that and no way to name the action on its button.
 *
 * Copy rule: the confirm button repeats the verb the user is about to commit
 * to — "Discard changes", "Delete catalog" — never "OK".
 *
 * A Radix `AlertDialog`: it opens on the safe button, so a stray Enter keeps
 * what is at stake, and a click on the scrim does nothing. Escape and the
 * cancel button both answer `onCancel`. On close, focus goes back to what
 * opened it (`useReturnFocus`).
 */
export function ConfirmDialog({
  open,
  title,
  body,
  confirmLabel,
  cancelLabel = 'Cancel',
  destructive = false,
  pending = false,
  error = null,
  onConfirm,
  onCancel,
}: {
  open: boolean
  title: string
  body: ReactNode
  confirmLabel: string
  cancelLabel?: string
  destructive?: boolean
  /** True while the confirmed action is in flight. Disables the confirm
   *  button so a second click can't fire a second request. */
  pending?: boolean
  /** Plain-text failure from the last confirm attempt, shown under the body. */
  error?: string | null
  onConfirm: () => void
  onCancel: () => void
}) {
  const onCloseAutoFocus = useReturnFocus(open)
  return (
    <AlertDialog.Root open={open} onOpenChange={(next) => !next && onCancel()}>
      <AlertDialog.Portal>
        <AlertDialog.Overlay className="fixed inset-0 z-50 grid place-items-center overscroll-contain bg-[var(--uno-scrim)] p-4 sm:p-8">
          {/* `max-h-full` and a scroll of its own. The body names what is at
              stake — a collection's title, the folders going with it — so it
              has no bounded height, and a panel taller than the viewport in a
              `place-items-center` grid is clipped at both ends with no way to
              reach either. */}
          <AlertDialog.Content
            onCloseAutoFocus={onCloseAutoFocus}
            className="bg-raised border-line-hi flex max-h-full w-full max-w-[440px] flex-col gap-5 overflow-y-auto overscroll-contain rounded-[20px] border p-6 outline-none"
          >
            <AlertDialog.Title className="type-display m-0 text-display">
              {title}
            </AlertDialog.Title>
            <AlertDialog.Description asChild>
              <div className="text-dim text-body-sm leading-relaxed">{body}</div>
            </AlertDialog.Description>
            {error && (
              <p
                role="alert"
                className="callout-danger type-data"
              >
                {error}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <AlertDialog.Cancel className="btn-ghost">{cancelLabel}</AlertDialog.Cancel>
              <Button kind={destructive ? 'danger' : 'primary'} onClick={onConfirm} disabled={pending} aria-live="polite">
                {confirmLabel}
              </Button>
            </div>
          </AlertDialog.Content>
        </AlertDialog.Overlay>
      </AlertDialog.Portal>
    </AlertDialog.Root>
  )
}
