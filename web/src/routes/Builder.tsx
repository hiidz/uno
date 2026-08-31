import { useRef, useState } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EditorGuardProvider, useEditorGuard } from '@/features/builder/EditorGuard'
import { ProfileMenu } from '@/features/builder/ProfileMenu'
import { usePublishedHeaderHeight } from '@/features/builder/stacked'
import { Workspace } from '@/features/builder/Workspace'
import { HomeSelectionProvider } from '@/features/home/HomeSelectionContext'
import { useHomeSelection } from '@/features/home/useHomeSelection'
import { useUnloadGuard } from '@/features/home/useUnloadGuard'
import { AddonURLButton, PushBanner, PushButton } from '@/features/push/PushControls'
import { usePush } from '@/features/push/usePush'

export interface BuilderProfile {
  profileIndex: number
  profileName: string
  /** Uno's addon URL for this profile, from `POST /api/profiles/select`.
   *  Optional because navigation state is the only source, and a history entry
   *  may not carry it. */
  manifestURL?: string
}

/**
 * The single builder page. Two regions, no tabs: a Library rail on the left
 * (every catalog and collection, yours and community — the source you pick
 * from) and a pane on the right holding one thing at a time — your home screen,
 * or the editor for whichever rail row is selected. Below `lg` the same two
 * regions stack into one scrolling document rather than becoming two screens —
 * see `stacked.ts`.
 *
 * Push is a header action — the commit for Home, so it lives where Home's state
 * is, whether or not Home is the pane's current occupant.
 *
 * **Two kinds of unsaved work, deliberately separate.** An editor's changes are
 * saved to the server; Home's are pushed to your TV. They're lost in different
 * ways, warn separately, and the header shows only Home's — an editor states
 * its own in the pane.
 */
export function Builder() {
  const location = useLocation()
  const profile = location.state as BuilderProfile | null

  // Only reachable by selecting a profile on /profiles, which hands the
  // profile down via navigation state — a direct or refreshed visit to this
  // URL has no state, so send the user back to pick a profile.
  if (!profile) {
    return <Navigate to="/profiles" replace />
  }

  return (
    <HomeSelectionProvider profileIndex={profile.profileIndex}>
      <EditorGuardProvider>
        <div className="flex min-h-svh flex-col lg:h-svh">
          <BuilderHeader profile={profile} />
          <Workspace profileIndex={profile.profileIndex} />
        </div>
      </EditorGuardProvider>
    </HomeSelectionProvider>
  )
}

function BuilderHeader({ profile }: { profile: BuilderProfile }) {
  const navigate = useNavigate()
  const home = useHomeSelection()
  const editor = useEditorGuard()
  const push = usePush(profile.profileIndex)
  const [confirmingLeave, setConfirmingLeave] = useState(false)
  const stickyRef = useRef<HTMLDivElement>(null)

  usePublishedHeaderHeight(stickyRef)

  // Both kinds of unsaved work block a page close, for the same reason: neither
  // survives it. Which one it was is a question the browser's own dialog can't
  // ask, so the two are simply OR-ed here.
  useUnloadGuard(home.isDirty || editor.dirty)

  function leave() {
    setConfirmingLeave(false)
    void navigate('/profiles')
  }

  /**
   * Leaving the page passes both guards, editor first.
   *
   * Two prompts rather than one merged dialog: they're about different things
   * lost in different ways, and a single sentence covering both would have to
   * be vague about each. The editor's is raised by `guard`; Home's is the
   * dialog below.
   */
  function requestLeave() {
    editor.guard(() => {
      if (home.isDirty) {
        setConfirmingLeave(true)
        return
      }
      leave()
    })
  }

  return (
    <>
      {/* Sticky below `lg`, where the page scrolls as one document: Push and
          the banner reporting its result are the two things that must not
          scroll away from someone halfway down a home screen. Above `lg` the
          shell is a fixed-height flex column and this band is already pinned.
          `z-30` sits under `Modal` and `ConfirmDialog`, which are `z-50`.

          Measured, not assumed: this band is what everything below `lg` pins
          and scrolls to the underside of. The header itself is now one row at a
          fixed height, but `PushBanner` is this wrapper's second child rather
          than the header's, and it mounts and unmounts while the page is open —
          so the band's height still moves and is still read from the DOM. */}
      <div ref={stickyRef} className="sticky top-0 z-30 shrink-0 lg:static">
        {/* One row that never wraps, at every width. It used to be `flex-wrap`,
            which cost 90px on a 390px screen and 133px on a 360px one, and made
            the height depend on the pending count — the only label here whose
            text length follows the data. The row now holds its height by
            letting the profile chip truncate instead. */}
        <header className="bg-raised border-line flex min-h-[53px] items-center gap-3 border-b px-4 py-2 lg:h-[53px] lg:gap-4 lg:px-5 lg:py-0">
          <span className="type-wordmark shrink-0 text-[14px]">Uno</span>

          {/* Switching profiles means going back through the picker —
              /configure is only reachable via navigation state, so there's
              nowhere else to re-select from. That makes this the page's only
              in-app exit, and therefore the one control that has to guard
              pending changes.

              Two chips, one at a time. Below `lg` it is the trigger for the
              header's only menu, which also carries the addon URL; above `lg`
              there is room for both controls and the chip stays the one-tap
              switch it has always been. `hidden` rather than a single chip
              styled twice, because `display: none` is what keeps the one that
              isn't on screen out of the accessibility tree as well. */}
          <ProfileMenu
            profileIndex={profile.profileIndex}
            profileName={profile.profileName}
            manifestURL={profile.manifestURL}
            onSwitchProfile={requestLeave}
            className="lg:hidden"
          />
          <button
            type="button"
            onClick={requestLeave}
            title="Switch profile"
            className="type-data border-line-hi text-dim hover:text-ink hover:border-dim hidden shrink-0 rounded-[2px] border px-2 py-[3px] text-[11px] whitespace-nowrap transition-colors lg:block"
          >
            profile {profile.profileIndex} · {profile.profileName}
          </button>

          {/* Grouped so the row doesn't reflow while PendingIndicator is still
              withholding itself during load. `shrink-0`: what the row runs out
              of room for is absorbed by the chip, not taken out of the status
              and the action. */}
          <div className="ml-auto flex shrink-0 items-center gap-3 lg:gap-4">
            <PendingIndicator />
            {profile.manifestURL && (
              <AddonURLButton url={profile.manifestURL} className="hidden lg:block" />
            )}
            <PushButton {...push} />
          </div>
        </header>

        <PushBanner {...push} />
      </div>

      <ConfirmDialog
        open={confirmingLeave}
        title="Discard unpushed changes?"
        body={
          <>
            {countLabel(home.pendingCount)} to your home screen{' '}
            {home.pendingCount === 1 ? "hasn't" : "haven't"} been pushed yet. Leaving discards{' '}
            {home.pendingCount === 1 ? 'it' : 'them'}.
          </>
        }
        confirmLabel="Discard and switch"
        cancelLabel="Stay here"
        destructive
        onConfirm={leave}
        onCancel={() => setConfirmingLeave(false)}
      />
    </>
  )
}

/**
 * Pending edits live only in browser memory, so without this the user has no
 * way to tell that what's on screen isn't what's on their TV. Sits beside the
 * Push button: information next to the action that resolves it.
 *
 * Below `lg` it is a dot and a count — 25px against the sentence's 139px, in a
 * row that has to fit five things. The dot alone is what a clean home screen
 * gets, and it is drawn rather than dropped: this returns `null` until
 * `home.ready`, so rendering nothing already means "not loaded yet", and a
 * silent clean state would be indistinguishable from one still loading.
 */
function PendingIndicator() {
  const home = useHomeSelection()
  if (!home.ready) return null

  const sentence = home.isDirty
    ? `${countLabel(home.pendingCount)} unpushed`
    : 'no unpushed changes'

  return (
    <span
      className={`type-data flex shrink-0 items-center gap-1.5 text-[11px] whitespace-nowrap lg:gap-2 ${
        home.isDirty ? 'text-dim' : 'text-dimmer'
      }`}
      title={
        home.isDirty
          ? 'These changes only exist in this tab until you push.'
          : 'Your home screen matches what was last pushed.'
      }
    >
      <span
        aria-hidden="true"
        className={`h-[6px] w-[6px] rounded-full ${home.isDirty ? 'bg-series' : 'bg-dimmer'}`}
      />

      {/* The words, for a screen reader, on the screens where they aren't
          drawn. Not `aria-label` on this span — a span is `role="generic"`,
          which ARIA forbids naming — and not `title`, which never opens on
          touch. `lg:hidden` rather than leaving it in place, so it isn't read
          twice over the visible sentence beside it.

          No `role="status"`: the count changes on every edit, and announcing
          each one interrupts the work that caused it. It is readable on
          request, not broadcast. */}
      <span className="sr-only lg:hidden">{sentence}</span>
      {home.isDirty && (
        <span aria-hidden="true" className="lg:hidden">
          {home.pendingCount}
        </span>
      )}
      <span className="hidden lg:inline">{sentence}</span>
    </span>
  )
}

function countLabel(count: number): string {
  return `${count} ${count === 1 ? 'change' : 'changes'}`
}
