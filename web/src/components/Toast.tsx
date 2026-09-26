import type { ToastMessage } from './useToast'

/** The message `useToast` holds, or nothing once it has cleared. */
export function Toast({ toast }: { toast: ToastMessage | null }) {
  if (!toast) return null
  return (
    <div
      role="status"
      className={`bg-raised-hi type-data rounded-full px-4 py-2 text-[13.5px] font-semibold shadow-[inset_0_0_0_1px_var(--uno-line-hi)] ${
        toast.tone === 'danger' ? 'text-danger' : 'text-ink'
      }`}
    >
      {toast.text}
    </div>
  )
}
