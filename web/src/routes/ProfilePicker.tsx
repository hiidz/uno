import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import { ApiError, fetchProfiles, queryKeys, selectProfile } from '@/api'
import type { NuvioProfile } from '@/api'

export function ProfilePicker() {
  const navigate = useNavigate()
  const [selecting, setSelecting] = useState<number | null>(null)

  const profiles = useQuery({
    queryKey: queryKeys.profiles(),
    queryFn: fetchProfiles,
  })

  const select = useMutation({
    mutationFn: (profileIndex: number) => selectProfile(profileIndex),
  })

  function choose(profile: NuvioProfile) {
    setSelecting(profile.profile_index)
    select.mutate(profile.profile_index, {
      // `manifest_url` rides along so the builder can show the addon URL
      // straight away instead of waiting for a first push to report it. It's
      // built server-side from SITE_BASE_URL, which the client is never told.
      onSuccess: (selected) => {
        navigate('/configure', {
          state: {
            profileIndex: profile.profile_index,
            profileName: profile.name,
            manifestURL: selected.manifest_url,
          },
        })
      },
      onError: () => setSelecting(null),
    })
  }

  const error =
    (profiles.error instanceof ApiError ? profiles.error.message : profiles.error?.message) ??
    (select.error instanceof ApiError ? select.error.message : select.error?.message)

  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <div className="flex w-full max-w-[340px] flex-col gap-6">
        <div className="flex flex-col gap-2">
          <h1 className="type-display m-0 text-[17px]">Pick a profile</h1>
          <p className="type-data text-dim text-[11.5px]">
            Everything you build belongs to the profile you pick.
          </p>
        </div>

        {error && <p className="type-data text-danger m-0 text-[11.5px]">{error}</p>}
        {profiles.isPending && (
          <p className="type-data text-dimmer m-0 text-[11.5px]">Loading profiles…</p>
        )}
        {profiles.data?.length === 0 && (
          <p className="type-data text-dimmer m-0 text-[11.5px]">
            This Nuvio account has no profiles. Create one in Nuvio first.
          </p>
        )}

        <div className="flex flex-col">
          {profiles.data?.map((profile) => (
            <button
              key={profile.id}
              type="button"
              disabled={selecting !== null}
              onClick={() => choose(profile)}
              className="border-line hover:bg-raised flex items-center gap-3 border-b px-1 py-3 text-left transition-colors disabled:opacity-50"
            >
              <span className="type-data text-dimmer w-4 text-[11px]">
                {profile.profile_index}
              </span>
              <span className="flex-1 text-[14px] font-medium">{profile.name}</span>
              {selecting === profile.profile_index && (
                <span className="type-data text-dimmer text-[10.5px]">Selecting…</span>
              )}
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}
