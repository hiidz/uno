import { withoutDeleted } from './pending'
import type { HomeState } from './pending'

/**
 * Drops the rows that no longer exist from the pending state and the
 * baseline (`withoutDeleted`), by setting each while the provider renders — the
 * guard the provider's own hydration uses, which settles in one more render.
 */
export function usePrunedHome(
  current: HomeState | null,
  baseline: HomeState,
  existing: ReadonlySet<string> | null,
  setCurrent: (state: HomeState | null) => void,
  setBaseline: (state: HomeState) => void,
): void {
  const prunedCurrent = withoutDeleted(current, existing)
  if (prunedCurrent !== current) setCurrent(prunedCurrent)
  const prunedBaseline = withoutDeleted(baseline, existing)
  if (prunedBaseline !== baseline) setBaseline(prunedBaseline)
}
