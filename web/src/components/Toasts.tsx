import { useCallback, useRef, useState, type ReactNode } from 'react'
import { Check, X } from 'lucide-react'
import { Toast } from 'radix-ui'
import { Icon } from './Icon'
import { TOAST_MS, ToastContext, withToast, type QueuedToast, type ToastInput, type ToastTone } from './toastQueue'

/** How long a closed toast stays mounted for its fade out. */
const EXIT_MS = 200

/**
 * The app's toasts (DESIGN.md, "Toast"; docs/frontend.md, UI rule 1): one
 * stack in the window's bottom-right corner, full width at a phone's foot,
 * shown through `useToasts().show`. A success or a notice leaves after
 * `TOAST_MS`, the timer pausing while the pointer or focus is on the stack or
 * the window is in the background; a failure stays until closed. × or a swipe
 * right closes any of them, and F8 moves focus to the stack.
 *
 * A screen with a bar along a phone's foot (the jump button, an editor's save
 * bar) sets `--toast-lift` to its height, so the stack sits above it.
 */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<QueuedToast[]>([])
  const nextID = useRef(0)

  const show = useCallback((toast: ToastInput) => {
    const id = nextID.current++
    setToasts((list) => withToast(list, { ...toast, id, open: true }))
  }, [])

  function close(id: number) {
    setToasts((list) => list.map((toast) => (toast.id === id ? { ...toast, open: false } : toast)))
    window.setTimeout(() => setToasts((list) => list.filter((toast) => toast.id !== id)), EXIT_MS)
  }

  return (
    <ToastContext.Provider value={show}>
      <Toast.Provider swipeDirection="right" label="Notifications">
        {children}
        {toasts.map((toast) => (
          <Toast.Root
            key={toast.id}
            open={toast.open}
            onOpenChange={(open) => !open && close(toast.id)}
            duration={toast.tone === 'danger' ? Infinity : TOAST_MS}
            type={toast.tone === 'danger' ? 'foreground' : 'background'}
            data-tone={toast.tone}
            className="toast"
          >
            <Mark tone={toast.tone} />
            <div className="min-w-0 flex-1">
              <Toast.Title className="text-body-sm font-semibold [overflow-wrap:anywhere]">{toast.text}</Toast.Title>
              {toast.detail && (
                <Toast.Description className="text-meta text-dim mt-0.5 [overflow-wrap:anywhere]">
                  {toast.detail}
                </Toast.Description>
              )}
            </div>
            <Toast.Close
              aria-label="Dismiss"
              className="tap text-dim hover:text-ink hover:bg-line grid h-[22px] w-8 shrink-0 place-items-center rounded-full transition-colors"
            >
              <Icon icon={X} size={14} />
            </Toast.Close>
          </Toast.Root>
        ))}
        <Toast.Viewport className="toast-viewport" />
      </Toast.Provider>
    </ToastContext.Provider>
  )
}

/** The round mark a toast leads with: a check, a !, or an i. */
function Mark({ tone }: { tone: ToastTone }) {
  return (
    <span aria-hidden="true" className="toast-mark">
      {tone === 'success' ? <Icon icon={Check} size={14} /> : tone === 'danger' ? '!' : 'i'}
    </span>
  )
}
