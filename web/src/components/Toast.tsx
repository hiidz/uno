import type { ToastMessage } from './useToast'

/** The message `useToast` holds, or nothing once it has cleared. */
export function Toast({ toast }: { toast: ToastMessage | null }) {
  if (!toast) return null
  return (
    <div
      role="status"
      className={`bg-raised type-data border px-3 py-2 text-[11px] ${
        toast.tone === 'danger' ? 'border-danger text-danger' : 'border-line text-ink'
      }`}
    >
      {toast.text}
    </div>
  )
}
