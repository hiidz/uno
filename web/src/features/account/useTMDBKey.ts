import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchServerConfig, fetchTMDBKey, queryKeys, removeTMDBKey, saveTMDBKey } from '@/api'
import type { TMDBKeyStatus } from '@/api'
import { clearKeyProblem } from './keyProblem'

/**
 * What the profile picker shows of the account's TMDB key: `checking` while
 * it is still finding out whether to ask, nothing on a server where every
 * account shares one (or when the account is refused, or when either answer
 * failed), `needed` while the account has saved none, and the key's last four
 * characters once it has.
 */
export type KeyStep = { kind: 'checking' } | { kind: 'none' } | { kind: 'needed' } | { kind: 'set'; last4: string }

/** The picker's key step. `refused` is the access refusal, which leaves
 *  nothing to ask for. */
export function useKeyStep(refused: boolean): KeyStep {
  const config = useQuery({
    queryKey: queryKeys.serverConfig(),
    queryFn: fetchServerConfig,
    staleTime: Infinity,
  })
  const asked = config.data?.tmdb_key_mode === 'per-account' && !refused
  const key = useQuery({
    queryKey: queryKeys.tmdbKey(),
    queryFn: fetchTMDBKey,
    enabled: asked,
  })
  return keyStep(checking(refused, config.isPending, key.isLoading), asked ? key.data : undefined)
}

/** Whether the picker is still finding out whether to ask for a key: the
 *  mode, or the key's status in per-account mode, hasn't come back. A refused
 *  account is asked nothing. A disabled query is never loading. */
function checking(refused: boolean, configPending: boolean, keyLoading: boolean): boolean {
  if (refused) return false
  return configPending || keyLoading
}

/** The step a key status puts the picker in; `undefined` is no status, or
 *  none to ask for. */
export function keyStep(isChecking: boolean, status: TMDBKeyStatus | undefined): KeyStep {
  if (isChecking) return { kind: 'checking' }
  if (!status) return { kind: 'none' }
  if (!status.set) return { kind: 'needed' }
  return { kind: 'set', last4: status.last4 }
}

/** Whether step holds the profiles: while a key is needed, and while it isn't
 *  known yet whether one is, so no profile opens the builder without one. */
export function holdsProfiles(step: KeyStep): boolean {
  return step.kind === 'checking' || step.kind === 'needed'
}

/** Saving and removing the key, each written straight into the key query
 *  so the picker changes step without asking again. A saved key also clears
 *  the builder's key problem. */
export function useKeyMutations() {
  const queryClient = useQueryClient()
  const save = useMutation({
    mutationFn: saveTMDBKey,
    onSuccess: (status) => {
      queryClient.setQueryData(queryKeys.tmdbKey(), status)
      clearKeyProblem()
    },
  })
  const remove = useMutation({
    mutationFn: removeTMDBKey,
    onSuccess: () => queryClient.setQueryData<TMDBKeyStatus>(queryKeys.tmdbKey(), { set: false }),
  })
  return { save, remove }
}

/** The id `TMDBKeyGate`'s heading takes, which the held profile cards point
 *  at to say why they are held. */
export const KEY_GATE_HEADING = 'tmdb-key-gate'
