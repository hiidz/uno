import { createContext, useContext } from 'react'

/** A finished action's outcome, a failure, or a notice that explains a move the app made. */
export type ToastTone = 'success' | 'danger' | 'notice'

export interface ToastInput {
  tone: ToastTone
  /** The headline: what happened. */
  text: string
  /** A failure's reason in the server's own words, on a dim second line. */
  detail?: string
}

export interface QueuedToast extends ToastInput {
  id: number
  open: boolean
}

/** The most toasts on screen at once. */
export const MAX_TOASTS = 3

/** How long a success or a notice stays; a failure stays until closed. */
export const TOAST_MS = 5000

/**
 * The list with `next` added last (newest at the bottom). Past `MAX_TOASTS`
 * the oldest success or notice goes first, so a run of quick successes never
 * pushes an unread failure off screen; when every toast is a failure, the
 * oldest goes. Toasts already closing don't count.
 */
export function withToast(list: QueuedToast[], next: QueuedToast): QueuedToast[] {
  const open = list.filter((toast) => toast.open)
  if (open.length < MAX_TOASTS) return [...list, next]
  const drop = open.find((toast) => toast.tone !== 'danger') ?? open[0]
  return [...list.filter((toast) => toast !== drop), next]
}

export const ToastContext = createContext<((toast: ToastInput) => void) | null>(null)

/** Shows a toast through the app's one `ToastProvider`. */
export function useToasts(): { show: (toast: ToastInput) => void } {
  const show = useContext(ToastContext)
  if (!show) throw new Error('useToasts needs a ToastProvider above it')
  return { show }
}
