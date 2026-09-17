import { useCallback, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { ProfileNotSelectedError, pushSelection, queryKeys } from '@/api'
import { toPushPayload } from '@/features/home/pending'
import { useHomeSelection } from '@/features/home/useHomeSelection'

/**
 * What the user is told after a push. The backend attempts Nuvio before writing
 * anything locally, so every ordinary failure collapses into one "nothing
 * changed" rather than a per-stage report.
 */
export type PushOutcome =
  | { kind: 'success'; manifestURL?: string }
  /** Anything ordinary: rejected input, Nuvio unreachable, either push
   *  refused. Nothing was written, so retrying costs nothing. */
  | { kind: 'failed' }
  /** The narrow case the atomicity guarantee doesn't cover — the local write
   *  failed after Nuvio had accepted the push, and the revert failed too. */
  | { kind: 'undo-failed' }
  /** No usable answer came back at all: a dropped connection, a proxy error
   *  page, a response that never parsed. We genuinely don't know whether the
   *  push landed, and must not claim nothing happened. */
  | { kind: 'unknown' }

export interface Push {
  push: () => void
  pushing: boolean
  outcome: PushOutcome | null
  dismiss: () => void
}

export function usePush(profileIndex: number): Push {
  const home = useHomeSelection()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [pushing, setPushing] = useState(false)
  const [outcome, setOutcome] = useState<PushOutcome | null>(null)

  // A ref, not the `pushing` state: state read inside the callback is the value
  // captured at render, so it can't reject a second call raised before React
  // re-renders. Two overlapping pushes clobber each other in Nuvio, so the
  // button's `disabled` alone isn't enough.
  const inFlight = useRef(false)

  const push = useCallback(() => {
    if (inFlight.current) return
    inFlight.current = true

    // Snapshotted before the await, and handed back to `markPushed` on
    // success: the user can keep editing while the push is in flight, and
    // those edits must stay pending rather than being acknowledged by a push
    // that never carried them.
    const sent = home.snapshot()

    setPushing(true)
    setOutcome(null)

    void (async () => {
      try {
        const result = await pushSelection(profileIndex, toPushPayload(sent))
        if (result.success) {
          home.markPushed(sent)
          // The cached selection responses are now stale — the server holds
          // what we just pushed. `staleTime` is 30s, so without this a profile
          // switch and return inside that window would re-hydrate the baseline
          // from pre-push data and the pushed changes would look undone. The
          // provider's hydration is guarded by `current !== null`, so the
          // refetch updates the lookup maps without touching pending edits.
          void queryClient.invalidateQueries({ queryKey: queryKeys.catalogSelection(profileIndex) })
          void queryClient.invalidateQueries({
            queryKey: queryKeys.collectionSelection(profileIndex),
          })
          // `pushed_version` lives on the owned-collection row too, and
          // `HomeSelectionContext`'s `collectionById` map lets the owned list
          // win over the selection response on id collision (it's built
          // second) — so without this, a collection that's both owned and
          // currently selected keeps showing its pre-push `pushed_version` and
          // the "changed since it was last pushed" line never clears.
          void queryClient.invalidateQueries({ queryKey: queryKeys.ownedCollections(profileIndex) })
          setOutcome({ kind: 'success', manifestURL: result.manifest_url })
        } else {
          setOutcome({ kind: result.undo_failed ? 'undo-failed' : 'failed' })
        }
      } catch (err) {
        // A 404 here means the profile slot was never selected — there's
        // nothing to retry, so send the user back to pick one.
        if (err instanceof ProfileNotSelectedError) {
          void navigate('/profiles', { replace: true })
          return
        }
        setOutcome({ kind: 'unknown' })
      } finally {
        inFlight.current = false
        setPushing(false)
      }
    })()
  }, [home, navigate, profileIndex, queryClient])

  const dismiss = useCallback(() => setOutcome(null), [])

  return { push, pushing, outcome, dismiss }
}
