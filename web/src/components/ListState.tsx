import type { ReactNode } from 'react'

/**
 * The loading / error / empty wrapper every list in the app goes through.
 * Shared rather than per-feature so the three states read identically in the
 * Library rail, the Home pane and Community. A list with no error to report
 * leaves `error` out, and one with no empty state of its own leaves `isEmpty`
 * out.
 *
 * Copy rules: errors say what happened and offer the next action — no
 * apology, no vagueness. An empty list is an invitation to act, so callers
 * pass real guidance rather than "No items".
 */
export function ListState({
  isLoading,
  error = null,
  errorLabel,
  onRetry,
  isEmpty = false,
  loadingLabel = 'Loading…',
  emptyLabel,
  children,
}: {
  isLoading: boolean
  error?: Error | null
  errorLabel?: string
  onRetry?: () => void
  isEmpty?: boolean
  loadingLabel?: string
  emptyLabel?: ReactNode
  children: ReactNode
}) {
  if (error) return <ListError label={errorLabel} error={error} onRetry={onRetry} />

  if (isLoading) {
    return <p className="type-data text-dimmer m-0 py-2 text-[12.5px]">{loadingLabel}</p>
  }

  if (isEmpty) {
    return <div className="type-data text-dimmer py-2 text-[12.5px]">{emptyLabel}</div>
  }

  return <>{children}</>
}

/** A list's failure: the headline, the server's own words, and a Retry. */
export function ListError({
  label,
  error,
  onRetry,
}: {
  label?: string
  error: Error
  onRetry?: () => void
}) {
  return (
    <div className="flex flex-col items-start gap-2 py-2">
      <p className="type-data text-danger m-0 text-[12.5px]">{label}</p>
      {/* The server's error is usually more specific than
          anything we'd synthesise, so surface it under the headline. */}
      <p className="type-data text-dimmer m-0 text-[12.5px]">{error.message}</p>
      {onRetry && (
        <button type="button" onClick={onRetry} className="btn-ghost">
          Retry
        </button>
      )}
    </div>
  )
}
