import { useEffect, useState } from 'react'

/** `value`, once it has held still for `delayMs`. Each change restarts the
 *  wait, so a burst of keystrokes settles into one update at the end. */
export function useDebounce<T>(value: T, delayMs: number): T {
  const [settled, setSettled] = useState(value)

  useEffect(() => {
    const timer = setTimeout(() => setSettled(value), delayMs)
    return () => clearTimeout(timer)
  }, [value, delayMs])

  return settled
}
