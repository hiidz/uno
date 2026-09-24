import { useEffect, useState } from 'react'

export interface ToastMessage {
  text: string
  tone: 'success' | 'danger'
}

/**
 * A short outcome message, drawn by `Toast`, that clears itself after 2.5s.
 * Auto-dismissing, unlike the push outcome strip: nothing it says needs a
 * decision, so there is nothing worth keeping on screen once it's been read.
 * Setting a new message restarts the timer.
 */
export function useToast() {
  const [toast, setToast] = useState<ToastMessage | null>(null)

  useEffect(() => {
    if (!toast) return
    const timer = window.setTimeout(() => setToast(null), 2500)
    return () => window.clearTimeout(timer)
  }, [toast])

  return [toast, setToast] as const
}
