import { useEffect, useState, type SyntheticEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import { ArrowRight, Lock, TriangleAlert } from 'lucide-react'
import { ApiError, fetchProfiles, queryKeys, selectProfile } from '@/api'
import type { NuvioProfile } from '@/api'
import { logout } from '@/auth'
import { TMDBKeyGate, TMDBKeyShelf } from '@/features/account/TMDBKey'
import { KEY_GATE_HEADING, holdsProfiles, useKeyStep } from '@/features/account/useTMDBKey'
import { EntryPage } from '@/components/EntryPage'
import { Fascia } from '@/components/Fascia'
import { Icon } from '@/components/Icon'
import { Wordmark } from '@/components/Wordmark'

/** How the picker shows a failed profile call. A 403 there is this server
 *  refusing the signed-in Nuvio account, which only its access policy answers:
 *  the picker says so in its own words. Anything else shows its message. */
function pickerFailure(error: Error | null): { refused: boolean; message?: string } {
  if (error instanceof ApiError && error.status === 403) return { refused: true }
  return { refused: false, message: error?.message }
}

/** Said in place of the profiles when this server doesn't admit the account.
 *  Signing in with another account, below, is the way on. */
function RefusedAccount({ shown }: { shown: boolean }) {
  if (!shown) return null
  return (
    <div className="bg-raised grid gap-2 rounded-2xl p-5">
      <p className="m-0 text-[17px] font-bold">This Nuvio account can&rsquo;t use this Uno.</p>
      <p className="text-dim m-0 max-w-[46ch] text-[15px]">
        Whoever runs it chooses who can sign in. Sign in with another account, or ask them to add this one.
      </p>
    </div>
  )
}

/** Whether a profile card is off: another is being chosen, or every card is
 *  held until the account saves a TMDB key (or until it is known whether it
 *  has one). */
function cardOff(isChoosing: boolean, isChosen: boolean, held: boolean): boolean {
  return held || (isChoosing && !isChosen)
}

/** What a profile card points at to say why it is as it is: the note naming
 *  the profile being chosen, or the TMDB key card asking for a key. */
function cardNote(isChoosing: boolean, gated: boolean, noteId: string): string | undefined {
  if (isChoosing) return noteId
  if (gated) return KEY_GATE_HEADING
  return undefined
}

/** Said on the card of a profile that uses profile 1's addons in Nuvio: it
 *  opens like any other, but Push is off for it (`usePushBlock`). */
function SharesAddonsNote({ profile }: { profile: NuvioProfile }) {
  if (!profile.uses_primary_addons) return null
  return <span className="text-dim truncate text-[13px]">Uses profile 1&rsquo;s addons in Nuvio · Push is off</span>
}

interface ProfileProps {
  profile: NuvioProfile
}

/**
 * The profile's picture as Nuvio's apps draw it: its image when it has one
 * (its own upload, or one of Nuvio's built-in avatars), else a circle in its
 * colour with its initial. Decorative: the card's name already says whose it
 * is. An image that fails to load leaves the coloured circle showing.
 */
function ProfileAvatar({ profile }: ProfileProps) {
  return (
    <span
      aria-hidden="true"
      style={{ backgroundColor: avatarColor(profile) }}
      className="text-ink relative grid size-10 shrink-0 place-items-center overflow-hidden rounded-full text-[18px] font-bold"
    >
      {initialOf(profile.name)}
      <AvatarImage url={profile.avatar_image_url} />
    </span>
  )
}

function AvatarImage({ url }: { url: string }) {
  if (!url) return null
  return (
    <img
      src={url}
      alt=""
      className="absolute inset-0 size-full object-cover"
      onError={hideImage}
    />
  )
}

function hideImage(event: SyntheticEvent<HTMLImageElement>) {
  event.currentTarget.hidden = true
}

/** Nuvio's own default colour for a profile that has none. */
const DEFAULT_AVATAR_COLOR = '#1E88E5'

function avatarColor(profile: NuvioProfile): string {
  if (!profile.avatar_color_hex) return DEFAULT_AVATAR_COLOR
  return profile.avatar_color_hex
}

function initialOf(name: string): string {
  return Array.from(name.trim())[0]?.toUpperCase() ?? ''
}

/** Said on the card of a profile with a PIN in Nuvio. Uno doesn't ask for
 *  the PIN: the badge only tells the profile apart the way Nuvio does. */
function PinBadge({ profile }: ProfileProps) {
  if (!profile.pin_enabled) return null
  return (
    <span className="type-sign text-dim flex items-center gap-1 text-[11px]">
      <Icon icon={Lock} size={12} />
      PIN in Nuvio
    </span>
  )
}

/** The card's accessible name names what `PinBadge` shows. */
function pinLabel(profile: NuvioProfile): string {
  if (!profile.pin_enabled) return ''
  return ', PIN in Nuvio'
}

/** The card's accessible name names what `SharesAddonsNote` shows. */
function sharesAddonsLabel(profile: NuvioProfile): string {
  if (!profile.uses_primary_addons) return ''
  return ', uses profile 1’s addons in Nuvio, Push is off'
}

export function ProfilePicker() {
  const navigate = useNavigate()
  const [selecting, setSelecting] = useState<number | null>(null)

  // The builder is a separate chunk that /configure loads lazily. Fetching it
  // while the picker is on screen means choosing a profile waits only on
  // `POST /api/profiles/select`, not on that and then the download.
  useEffect(() => {
    void import('./Builder')
  }, [])

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
            sharesAddons: profile.uses_primary_addons,
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

  const failure = pickerFailure(profiles.error ?? select.error)
  const error = failure.message
  // On a server where each account brings its own TMDB key, the profiles are
  // held until this account has saved one, and while that isn't known yet.
  const keyStep = useKeyStep(failure.refused)
  const held = holdsProfiles(keyStep)

  const choosingName = profiles.data?.find((p) => p.profile_index === selecting)?.name ?? 'that profile'
  const noteId = 'profiles-note'
  const isChoosing = selecting !== null

  return (
    <EntryPage
      wide
      title={<>Who&rsquo;s watching?</>}
    >
      {error && (
        <p role="alert" className="mb-4 grid grid-cols-[16px_minmax(0,1fr)] gap-x-2 text-[14px] leading-[20px]">
          <Icon icon={TriangleAlert} className="text-danger mt-0.5" />
          <span>{error}</span>
        </p>
      )}

      <div>
        {profiles.isPending && (
          <>
            <ul
              aria-busy="true"
              aria-label="Loading your profiles"
              className="m-0 grid list-none grid-cols-2 gap-4 p-0 max-sm:grid-cols-1"
            >
              {[0, 1].map((i) => (
                <li
                  key={i}
                  aria-hidden="true"
                  className="bg-raised flex h-[168px] flex-col overflow-hidden rounded-2xl"
                >
                  <span className="bg-line h-2.5" />
                  <span className="bg-line mt-auto mb-5 ml-4 h-6 w-2/5 rounded-full" />
                </li>
              ))}
            </ul>
            <p className="text-dim pt-4 text-[14px]">Getting your profiles from Nuvio…</p>
          </>
        )}

        <RefusedAccount shown={failure.refused} />
        <TMDBKeyGate step={keyStep} />

        {profiles.data?.length === 0 && (
          <div className="bg-raised grid gap-2 rounded-2xl p-5">
            <p className="m-0 text-[17px] font-bold">This account has no profiles yet.</p>
            <p className="text-dim m-0 max-w-[46ch] text-[15px]">
              Add one in Nuvio, then come back.
            </p>
          </div>
        )}

        {profiles.data && profiles.data.length > 0 && (
          <>
            <ul className="m-0 grid list-none grid-cols-2 gap-4 p-0 max-sm:grid-cols-1">
              {profiles.data.map((profile) => {
                const isChosen = selecting === profile.profile_index
                const isOff = cardOff(isChoosing, isChosen, held)
                return (
                  <li key={profile.id}>
                    <button
                      type="button"
                      disabled={isOff}
                      aria-disabled={isOff || undefined}
                      aria-describedby={cardNote(isChoosing, keyStep.kind === 'needed', noteId)}
                      aria-label={`Profile ${profile.profile_index}, ${profile.name}${pinLabel(profile)}${sharesAddonsLabel(profile)}${isChosen ? ', signing in' : ''}`}
                      onClick={() => choose(profile)}
                      className={`group flex h-[168px] w-full flex-col overflow-hidden rounded-2xl text-left transition-[background-color,opacity] duration-200 ease-[var(--uno-ease)] ${
                        isChosen
                          ? 'bg-line-hi'
                          : isOff
                            ? 'bg-raised opacity-45'
                            : 'bg-raised-hi hover:bg-line-hi'
                      }`}
                    >
                      {/* A membership card: the shop's fascia across the top,
                          the slot Nuvio shows it in, the member's picture and
                          name as Nuvio shows them. */}
                      <Fascia className="h-2.5 shrink-0" />
                      <span className="flex min-h-0 flex-1 flex-col px-4 pt-3.5 pb-4">
                        <span className="flex items-center justify-between gap-3">
                          <Wordmark className="text-dim h-[11px] w-auto" />
                          <span className="flex items-center gap-2">
                            <PinBadge profile={profile} />
                            <span className="type-sign text-dim text-[11px]">Profile {profile.profile_index}</span>
                          </span>
                        </span>
                        <span className="mt-auto flex min-w-0 items-center gap-3">
                          <ProfileAvatar profile={profile} />
                          <span className="truncate text-[28px] leading-tight font-bold">{profile.name}</span>
                        </span>
                        <SharesAddonsNote profile={profile} />
                        <span className="mt-2 flex items-center justify-between gap-3 text-[14px]">
                          <span className={isChosen ? 'text-ink' : 'text-dim'}>
                            {isChosen ? 'Signing in…' : 'Open its home screen'}
                          </span>
                          {!isOff && (
                            <span className="bg-ink text-sign-ink grid size-8 shrink-0 place-items-center rounded-full">
                              <Icon icon={ArrowRight} />
                            </span>
                          )}
                        </span>
                      </span>
                    </button>
                  </li>
                )
              })}
            </ul>
            {isChoosing && (
              <p id={noteId} className="text-dim pt-5 text-[14px]">
                Signing in as {choosingName}…
              </p>
            )}
          </>
        )}
      </div>

      <TMDBKeyShelf step={keyStep} />

      <div className="pt-6">
        <button
          type="button"
          onClick={switchAccount}
          disabled={isChoosing}
          aria-disabled={isChoosing || undefined}
          aria-describedby={isChoosing ? noteId : undefined}
          className="btn-ghost -ml-3"
        >
          Sign in with a different account
        </button>
      </div>
    </EntryPage>
  )
}
