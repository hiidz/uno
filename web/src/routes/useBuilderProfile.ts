import { useQuery } from '@tanstack/react-query'
import { ApiError, fetchProfiles, queryKeys, selectProfile } from '@/api'

/** The profile the builder is open on. */
export interface BuilderProfile {
  profileIndex: number
  profileName: string
  /** Uno's addon URL for this profile, from `POST /api/profiles/select`. */
  manifestURL: string
  /** The profile uses profile 1's addons in Nuvio, so Push is off for it
   *  (`usePushBlock`). */
  sharesAddons: boolean
}

export type BuilderProfileState =
  | { status: 'loading' }
  /** The account has no Nuvio profile in this slot. */
  | { status: 'missing' }
  | { status: 'failed'; retry: () => void }
  | { status: 'ready'; profile: BuilderProfile }

/**
 * The builder's profile, from its slot in the URL: its name and addon setting
 * from the account's profile list, and its addon URL from selecting it, which
 * also creates Uno's row for a slot visited for the first time — so nothing
 * profile-scoped is asked for until it answers. Coming from the picker, both
 * are cached already.
 */
export function useBuilderProfile(profileIndex: number): BuilderProfileState {
  const profiles = useQuery({ queryKey: queryKeys.profiles(), queryFn: fetchProfiles })
  const selected = useQuery({
    queryKey: queryKeys.selectedProfile(profileIndex),
    queryFn: () => selectProfile(profileIndex),
    staleTime: Infinity,
  })

  const nuvio = profiles.data?.find((p) => p.profile_index === profileIndex)
  // Select answers a slot the account doesn't have with a 400.
  const refused = selected.error instanceof ApiError && selected.error.status === 400
  if ((profiles.data && !nuvio) || refused) return { status: 'missing' }
  if (profiles.isError || selected.isError) {
    return {
      status: 'failed',
      retry: () => {
        if (profiles.isError) void profiles.refetch()
        if (selected.isError) void selected.refetch()
      },
    }
  }
  if (!nuvio || !selected.data) return { status: 'loading' }
  return {
    status: 'ready',
    profile: {
      profileIndex,
      profileName: nuvio.name,
      manifestURL: selected.data.manifest_url,
      sharesAddons: nuvio.uses_primary_addons,
    },
  }
}
