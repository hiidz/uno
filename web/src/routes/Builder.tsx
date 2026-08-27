import { useState } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EditorGuardProvider, useEditorGuard } from '@/features/builder/EditorGuard'
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
 * or the editor for whichever rail row is selected.
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
      <header className="bg-raised border-line flex h-[53px] shrink-0 items-center gap-4 border-b px-5">
        <span className="type-wordmark text-[14px]">Uno</span>

        {/* Switching profiles means going back through the picker — /configure
            is only reachable via navigation state, so there's nowhere else to
            re-select from. That makes this the page's only in-app exit, and
            therefore the one control that has to guard pending changes. */}
        <button
          type="button"
          onClick={requestLeave}
          title="Switch profile"
          className="type-data border-line-hi text-dim hover:text-ink hover:border-dim rounded-[2px] border px-2 py-[3px] text-[11px] transition-colors"
        >
          profile {profile.profileIndex} · {profile.profileName}
        </button>

        {/* Grouped so the row doesn't reflow while PendingIndicator is still
            withholding itself during load. */}
        <div className="ml-auto flex items-center gap-4">
          <PendingIndicator />
          {profile.manifestURL && <AddonURLButton url={profile.manifestURL} />}
          <PushButton {...push} />
        </div>
      </header>

      <PushBanner {...push} />

      <ConfirmDialog
        open={confirmingLeave}
        title="Discard unpushed changes?"
        body={
          <>
            You have {countLabel(home.pendingCount)} to your home screen that {
              home.pendingCount === 1 ? 'has' : 'have'
            } never been pushed. Leaving this page discards {home.pendingCount === 1 ? 'it' : 'them'}
            {' '}— nothing is saved until you push.
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
 */
function PendingIndicator() {
  const home = useHomeSelection()
  if (!home.ready) return null

  return (
    <span
      className={`type-data flex items-center gap-2 text-[11px] ${
        home.isDirty ? 'text-dim' : 'text-dimmer'
      }`}
      title={
        home.isDirty
          ? 'These changes are only in this browser tab until you push.'
          : 'Your home screen matches what was last pushed.'
      }
    >
      <span
        aria-hidden="true"
        className={`h-[6px] w-[6px] rounded-full ${home.isDirty ? 'bg-series' : 'bg-dimmer'}`}
      />
      {home.isDirty ? `${countLabel(home.pendingCount)} unpushed` : 'no unpushed changes'}
    </span>
  )
}

function countLabel(count: number): string {
  return `${count} ${count === 1 ? 'change' : 'changes'}`
}
