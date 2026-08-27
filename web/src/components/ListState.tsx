import type { ReactNode } from 'react'

/**
 * The loading / error / empty wrapper every list in the app goes through.
 * Shared rather than per-feature so the three states read identically in the
 * Library rail, the Home pane, and the Manage overlays.
 *
 * Copy rules, per .ref: errors say what happened and offer the next action —
 * no apology, no vagueness. An empty list is an invitation to act, so callers
 * pass real guidance rather than "No items".
 */
export function ListState({
  isLoading,
  error,
  isEmpty,
  loadingLabel = 'Loading…',
  emptyLabel,
  errorLabel,
  onRetry,
  children,
}: {
  isLoading: boolean
  error: Error | null
  isEmpty: boolean
  loadingLabel?: string
  emptyLabel: ReactNode
  errorLabel: string
  onRetry?: () => void
  children: ReactNode
}) {
  if (error) {
    return (
      <div className="flex flex-col items-start gap-2 py-2">
        <p className="type-data text-danger m-0 text-[11px]">{errorLabel}</p>
        {/* The server's plain-text body is usually more specific than
            anything we'd synthesise, so surface it under the headline. */}
        <p className="type-data text-dimmer m-0 text-[10.5px]">{error.message}</p>
        {onRetry && (
          <button type="button" onClick={onRetry} className="btn-ghost">
            Retry
          </button>
        )}
      </div>
    )
  }

  if (isLoading) {
    return <p className="type-data text-dimmer m-0 py-2 text-[11px]">{loadingLabel}</p>
  }

  if (isEmpty) {
    return <div className="type-data text-dimmer py-2 text-[11px]">{emptyLabel}</div>
  }

  return <>{children}</>
}
