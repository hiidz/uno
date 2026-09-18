import { useRef, useState } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { Segmented } from '@/components/fields'
import { EditorGuardProvider, useEditorGuard } from '@/features/builder/EditorGuard'
import { ProfileMenu } from '@/features/builder/ProfileMenu'
import { usePublishedHeaderHeight } from '@/features/builder/stacked'
import { Workspace } from '@/features/builder/Workspace'
import { CommunityView } from '@/features/community/CommunityView'
import { HomeSelectionProvider } from '@/features/home/HomeSelectionContext'
import { useHomeSelection } from '@/features/home/useHomeSelection'
import { useUnloadGuard } from '@/features/home/useUnloadGuard'
import { AddonURLButton, ChangesStrip, PushBanner, PushButton } from '@/features/push/PushControls'
import { usePush } from '@/features/push/usePush'
import { pluralCount } from '@/lib/plural'

export interface BuilderProfile {
  profileIndex: number
  profileName: string
  /** Uno's addon URL for this profile, from `POST /api/profiles/select`.
   *  Optional because navigation state is the only source, and a history entry
   *  may not carry it. */
  manifestURL?: string
}

type Tab = 'workspace' | 'community'

/**
 * The single builder page: a Workspace tab (two regions — a Library rail on
 * the left, every catalog and collection you own, and a pane on the right
 * holding your home screen or the editor for whichever rail row is selected)
 * and a Community tab (everyone else's public catalogs and collections,
 * browsed and taken). Below `lg` the Workspace
 * tab's own two regions stack into one scrolling document rather than
 * becoming two screens — see `stacked.ts`; Community is already one column at
 * every width, so it needs no such treatment.
 *
 * Push is a header action, outside both tabs — the commit for Home, so it
 * lives where Home's state is, regardless of which tab is open.
 *
 * **Two kinds of unsaved work, deliberately separate.** An editor's changes are
 * saved to the server; Home's are pushed to your TV. They're lost in different
 * ways, warn separately, and the header shows only Home's — an editor states
 * its own in the pane.
 */
export function Builder() {
  const location = useLocation()
  const profile = location.state as BuilderProfile | null
  const [tab, setTab] = useState<Tab>('workspace')

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
          <BuilderHeader profile={profile} tab={tab} onTabChange={setTab} />
          {tab === 'workspace' ? (
            <Workspace profileIndex={profile.profileIndex} />
          ) : (
            <CommunityView profileIndex={profile.profileIndex} />
          )}
        </div>
      </EditorGuardProvider>
    </HomeSelectionProvider>
  )
}

function BuilderHeader({
  profile,
  tab,
  onTabChange,
}: {
  profile: BuilderProfile
  tab: Tab
  onTabChange: (tab: Tab) => void
}) {
  const navigate = useNavigate()
  const home = useHomeSelection()
  const editor = useEditorGuard()
  const push = usePush(profile.profileIndex)
  const [confirmingLeave, setConfirmingLeave] = useState(false)
  const [changesOpen, setChangesOpen] = useState(false)
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

  /**
   * Switching tabs unmounts `Workspace`, and with it any open editor — so this
   * goes through the same guard as leaving the page, minus the pending-Home
   * prompt: Home's own state lives in `HomeSelectionProvider`, above both
   * tabs, and switching tabs doesn't touch it.
   *
   * Clears `dirty` itself, unlike `requestLeave` — leaving the page tears down
   * `EditorGuardProvider` along with everything else, but this provider spans
   * both tabs, so a stale `true` left over from the editor that just got
   * discarded would wrongly guard the *next* exit, with nothing left to lose.
   */
  function requestTabChange(next: Tab) {
    if (next === tab) return
    editor.guard(() => {
      editor.setDirty(false)
      onTabChange(next)
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
        {/* One row that never wraps, at every width, so the header's height
            stays fixed regardless of the pending count — the only label here
            whose text length follows the data. The profile chip truncates
            instead of letting the row wrap. */}
        <header className="bg-raised border-line flex min-h-[53px] items-center gap-3 border-b px-4 py-2 lg:h-[53px] lg:gap-4 lg:px-5 lg:py-0">
          <span className="type-wordmark shrink-0 text-[14px]">Uno</span>

          {/* Switching profiles means going back through the picker —
              /configure is only reachable via navigation state, so there's
              nowhere else to re-select from. That makes this the page's only
              way to leave it entirely, and along with the Community tab below,
              one of the two controls that have to guard pending editor
              changes.

              One chip, whose shape follows the breakpoint internally — see
              `ProfileMenu`. Below `lg` it is the trigger for the header's
              only menu, which also carries the addon URL; above `lg` there is
              room for both controls and the chip is a plain, one-tap switch. */}
          <ProfileMenu
            profileIndex={profile.profileIndex}
            profileName={profile.profileName}
            manifestURL={profile.manifestURL}
            onSwitchProfile={requestLeave}
          />

          {/* Above `lg` only — the header row is measured to fit exactly what
              it holds at phone width (see the touch-target note below), and a
              third control has no room there. Below `lg` the same tabs get
              their own row underneath, matching DESIGN.md's two-row phone
              top bar. */}
          <div className="hidden lg:block">
            <BuilderTabs tab={tab} onChange={requestTabChange} />
          </div>

          {/* Grouped so the row doesn't reflow while PendingIndicator is still
              withholding itself during load. `shrink-0`: what the row runs out
              of room for is absorbed by the chip, not taken out of the status
              and the action. */}
          <div className="ml-auto flex shrink-0 items-center gap-3 lg:gap-4">
            <PendingIndicator open={changesOpen} onToggle={() => setChangesOpen((o) => !o)} />
            {profile.manifestURL && (
              <AddonURLButton url={profile.manifestURL} className="hidden lg:block" />
            )}
            <PushButton {...push} />
          </div>
        </header>

        <div className="border-line bg-raised border-b px-4 py-2 lg:hidden">
          <BuilderTabs tab={tab} onChange={requestTabChange} />
        </div>

        <ChangesStrip
          changes={home.changes}
          open={changesOpen && home.isDirty}
          onHide={() => setChangesOpen(false)}
        />
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
 *
 * **Opens the list of changes.** While dirty, this is the toggle for
 * `ChangesStrip` — the count and the sentences it names must never disagree,
 * so the one control that states the count is also the one that opens the
 * list behind it. Clean, there's nothing to open, so it stays a plain,
 * unclickable span.
 */
function PendingIndicator({ open, onToggle }: { open: boolean; onToggle: () => void }) {
  const home = useHomeSelection()
  if (!home.ready) return null

  const sentence = home.isDirty
    ? `${countLabel(home.pendingCount)} unpushed`
    : 'no unpushed changes'

  const content = (
    <>
      <span
        aria-hidden="true"
        className={`h-[6px] w-[6px] ${home.isDirty ? 'bg-pending' : 'bg-dimmer'}`}
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
    </>
  )

  const className = `type-data flex shrink-0 items-center gap-1.5 text-[11px] whitespace-nowrap transition-colors lg:gap-2 ${
    home.isDirty ? 'text-dim' : 'text-dimmer'
  }`

  if (!home.isDirty) {
    return (
      <span className={className} title="Your home screen matches what was last pushed.">
        {content}
      </span>
    )
  }

  return (
    <button
      type="button"
      onClick={onToggle}
      aria-expanded={open}
      aria-label={`${sentence}. ${open ? 'Hide' : 'Show'} the list of changes.`}
      title="These changes only exist in this tab until you push."
      className={`${className} hover:text-ink`}
    >
      {content}
    </button>
  )
}

function countLabel(count: number): string {
  return pluralCount(count, 'change')
}

/** The Workspace / Community switch, factored out only because it's rendered
 *  twice — inline in the header above `lg`, on its own row below it. */
function BuilderTabs({ tab, onChange }: { tab: Tab; onChange: (tab: Tab) => void }) {
  return (
    <Segmented
      ariaLabel="Builder section"
      value={tab}
      onChange={onChange}
      options={[
        { value: 'workspace', label: 'Workspace' },
        { value: 'community', label: 'Community' },
      ]}
    />
  )
}
