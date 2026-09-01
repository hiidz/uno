import { useEffect, useRef } from 'react'
import type { ReactNode } from 'react'

/**
 * A confirmation the user has to answer before something irreversible happens.
 *
 * Not `window.confirm`: this app has to say *what* is at stake ("3 unpushed
 * changes", "removes it for everyone using it"), and a native dialog gives no
 * room for that and no way to name the action on its button.
 *
 * Copy rule: the confirm button repeats the verb the user is about to commit
 * to — "Discard changes", "Delete catalog" — never "OK".
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
  const confirmRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    if (!open) return
    confirmRef.current?.focus()
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') onCancel()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [open, onCancel])

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center overscroll-contain bg-[rgba(4,5,7,0.72)] p-4 sm:p-8"
      onClick={(event) => {
        if (event.target === event.currentTarget) onCancel()
      }}
    >
      {/* `max-h-full` and a scroll of its own, the same shape `Modal` uses. The
          body names what is at stake — a collection's title, the folders going
          with it — so it has no bounded height, and a panel taller than the
          viewport in a `place-items-center` grid is clipped at both ends with
          no way to reach either. */}
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="confirm-title"
        className="bg-ground border-line-hi flex max-h-full w-full max-w-[420px] flex-col gap-5 overflow-y-auto overscroll-contain border p-6"
      >
        <h2 id="confirm-title" className="type-display m-0 text-[14px]">
          {title}
        </h2>
        <div className="text-dim text-[13px] leading-relaxed">{body}</div>
        {error && (
          <p className="type-data text-danger border-danger m-0 border-l-2 pl-3 text-[11px] leading-[1.45]">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-2">
          <button type="button" onClick={onCancel} className="btn-ghost">
            {cancelLabel}
          </button>
          <button
            ref={confirmRef}
            type="button"
            onClick={onConfirm}
            disabled={pending}
            className="btn-primary"
            style={
              destructive
                ? { background: 'var(--uno-danger)', borderColor: 'var(--uno-danger)', color: '#fff' }
                : undefined
            }
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
