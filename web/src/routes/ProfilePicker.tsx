import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import { ArrowRight, TriangleAlert } from 'lucide-react'
import { ApiError, fetchProfiles, queryKeys, selectProfile } from '@/api'
import type { NuvioProfile } from '@/api'
import { logout } from '@/auth'
import { Icon } from '@/components/Icon'

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

  async function switchAccount() {
    if (selecting !== null) return
    await logout()
    navigate('/login')
  }

  const error =
    (profiles.error instanceof ApiError ? profiles.error.message : profiles.error?.message) ??
    (select.error instanceof ApiError ? select.error.message : select.error?.message)

  const choosingName = profiles.data?.find((p) => p.profile_index === selecting)?.name ?? 'that profile'
  const noteId = 'profiles-note'
  const isChoosing = selecting !== null

  return (
    <div className="min-h-svh px-4">
      <main className="mx-auto w-full max-w-[560px] pt-[18vh] pb-16 max-sm:pt-[12vh] max-sm:pb-40">
        <header className="border-line mb-8 grid gap-4 border-b pb-8">
          <h1 className="type-wordmark m-0 text-[40px] leading-none max-sm:text-[28px]">Uno</h1>
          <h2 className="type-display m-0 text-[20px] leading-[28px]">Who&rsquo;s watching?</h2>
          <p className="text-dim m-0 max-w-[44ch] text-[15px] leading-[22px]">
            Each profile has its own home screen. Pick yours to start building it.
          </p>
        </header>

        {error && (
          <p role="alert" className="mb-4 grid grid-cols-[16px_minmax(0,1fr)] gap-x-2 text-[14px] leading-[20px]">
            <Icon icon={TriangleAlert} className="text-danger mt-0.5" />
            <span>{error}</span>
          </p>
        )}

        <div className="-mt-8">
          {profiles.isPending && (
            <>
              <ul aria-busy="true" aria-label="Loading your profiles" className="m-0 list-none p-0">
                {[0, 1, 2, 3].map((i) => (
                  <li
                    key={i}
                    aria-hidden="true"
                    className="border-line grid min-h-14 grid-cols-[minmax(0,1fr)_minmax(0,1fr)] items-center gap-x-8 border-b py-3 pr-3 max-sm:grid-cols-[88px_minmax(0,1fr)] max-sm:gap-x-3"
                  >
                    <span className="bg-line ml-auto h-3.5 w-11" />
                    <span className="bg-line h-3.5 w-2/5" />
                  </li>
                ))}
              </ul>
              <p className="text-dim pt-4 text-[13px]">Getting your profiles from Nuvio…</p>
            </>
          )}

          {profiles.data?.length === 0 && (
            <div className="border-line grid gap-3 border-b pb-6">
              <p className="m-0 text-[16px] font-medium">This account has no profiles yet.</p>
              <p className="text-dim m-0 max-w-[46ch]">Add one in Nuvio on your TV, then come back.</p>
            </div>
          )}

          {profiles.data && profiles.data.length > 0 && (
            <>
              <ul className="m-0 list-none p-0">
                {profiles.data.map((profile) => {
                  const isC = selecting === profile.profile_index
                  const isOff = isChoosing && !isC
                  return (
                    <li key={profile.id}>
                      <button
                        type="button"
                        disabled={isOff}
                        aria-disabled={isOff || undefined}
                        aria-describedby={isChoosing ? noteId : undefined}
                        aria-label={`Profile ${profile.profile_index}, ${profile.name}${isC ? ', signing in' : ''}`}
                        onClick={() => choose(profile)}
                        className={`group grid w-full min-h-14 grid-cols-[minmax(0,1fr)_minmax(0,1fr)] items-center gap-x-8 border-b py-3 pr-3 text-left transition-colors max-sm:grid-cols-[88px_minmax(0,1fr)] max-sm:gap-x-3 ${
                          isC ? 'bg-raised border-line' : 'border-line hover:bg-raised disabled:hover:bg-transparent'
                        }`}
                      >
                        <span
                          className={`type-eyebrow text-right ${isOff ? 'text-dimmer' : ''}`}
                        >
                          Profile
                          <b className={`type-data ml-1.5 text-[15px] font-medium tracking-normal normal-case ${isOff ? 'text-dimmer' : 'text-ink'}`}>
                            {profile.profile_index}
                          </b>
                        </span>
                        <span className="flex min-w-0 items-center justify-between gap-3">
                          <span className={`text-[16px] leading-[22px] font-medium ${isOff ? 'text-dimmer' : ''}`}>
                            {profile.name}
                          </span>
                          <span
                            className={`inline-flex items-center gap-2 text-[13px] ${
                              isC ? 'text-ink' : 'text-dim group-hover:text-ink'
                            }`}
                          >
                            {isC ? 'Signing in…' : isOff ? '' : <Icon icon={ArrowRight} />}
                          </span>
                        </span>
                      </button>
                    </li>
                  )
                })}
              </ul>
              <p id={noteId} className="text-dim pt-4 text-[13px]">
                {isChoosing
                  ? `Signing in as ${choosingName}. The other profiles wait until that’s done.`
                  : 'Slot numbers match your TV.'}
              </p>
            </>
          )}
        </div>

        <div className="pt-6">
          <button
            type="button"
            onClick={switchAccount}
            disabled={isChoosing}
            aria-disabled={isChoosing || undefined}
            aria-describedby={isChoosing ? noteId : undefined}
            className="btn-quiet px-0"
          >
            Sign in with a different account
          </button>
        </div>
      </main>
    </div>
  )
}
