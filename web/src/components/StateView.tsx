import type { ReactNode } from 'react'

type Size = 'default' | 'compact'

/** Each size's line: the default a list's own, the compact one a field note's. */
const LINE: Record<Size, string> = {
  default: 'type-data m-0 text-[12.5px]',
  compact: 'type-data m-0 max-w-[var(--w-entry)] text-[13px] leading-[1.45]',
}

/**
 * The loading / error / empty wrapper every view of fetched data goes through.
 * Shared rather than per-feature so the three states read identically in the
 * Library rail, the Home pane, Community and the pickers. A view with no error
 * to report leaves `error` out, and one with no empty state of its own leaves
 * `isEmpty` out.
 *
 * Copy rules: errors say what happened and offer the next action — no
 * apology, no vagueness. An empty view is an invitation to act, so callers
 * pass real guidance rather than "No items".
 *
 * A view with a `skeleton` draws it while loading, rows shaped like its own
 * so nothing moves when they fill in, and keeps `loadingLabel` for screen
 * readers only.
 *
 * `size="compact"` is for pickers and popovers: each state is one line set
 * like a field's note, without the list's padding, and an error is its
 * headline alone.
 */
export function StateView({
  isLoading,
  error = null,
  errorLabel,
  onRetry,
  isEmpty = false,
  loadingLabel = 'Loading…',
  skeleton,
  emptyLabel,
  size = 'default',
  children,
}: {
  isLoading: boolean
  error?: Error | null
  errorLabel?: string
  onRetry?: () => void
  isEmpty?: boolean
  loadingLabel?: string
  skeleton?: ReactNode
  emptyLabel?: ReactNode
  size?: Size
  children: ReactNode
}) {
  const pad = size === 'default' ? 'py-2' : ''

  if (error) return <StateError label={errorLabel} error={error} onRetry={onRetry} size={size} />

  if (isLoading && skeleton) {
    return (
      <>
        <p className="sr-only">{loadingLabel}</p>
        <div aria-hidden="true">{skeleton}</div>
      </>
    )
  }

  if (isLoading) {
    return <p className={`${LINE[size]} text-dimmer ${pad}`}>{loadingLabel}</p>
  }

  if (isEmpty) {
    return <div className={`${LINE[size]} text-dimmer ${pad}`}>{emptyLabel}</div>
  }

  return <>{children}</>
}

/**
 * A failure: the headline, the server's own words, and a Retry. The compact
 * size keeps the headline and the Retry.
 */
export function StateError({
  label,
  error,
  onRetry,
  size = 'default',
}: {
  label?: string
  error: Error
  onRetry?: () => void
  size?: Size
}) {
  return (
    <div className={`flex flex-col items-start ${size === 'default' ? 'gap-2 py-2' : 'gap-1'}`}>
      <p className={`${LINE[size]} text-danger`}>{label}</p>
      {/* The server's error is usually more specific than
          anything we'd synthesise, so surface it under the headline. */}
      {size === 'default' && <p className={`${LINE[size]} text-dimmer`}>{error.message}</p>}
      {onRetry && (
        <button type="button" onClick={onRetry} className={size === 'default' ? 'btn-ghost' : 'btn-ghost btn-sm'}>
          Retry
        </button>
      )}
    </div>
  )
}
