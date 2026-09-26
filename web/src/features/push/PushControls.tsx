import { Check, Copy, Link, TriangleAlert, Tv, X } from 'lucide-react'
import { Icon } from '@/components/Icon'
import type { HomeChange } from '@/features/home/changes'
import type { Push } from './usePush'
import { useCopy } from './useCopy'

/**
 * The Push button. **Not** disabled when there is nothing pending: push is also
 * the only way to reconcile after the `undo-failed` case, where Nuvio ends up
 * ahead of Uno's vault with no local edit left to light up `isDirty`. Disabling
 * it on a clean home screen would remove that recovery path.
 *
 * It *is* disabled while a push is in flight. Two overlapping pushes mean two
 * interleaved pull-then-push cycles against Nuvio's collections blob, and the
 * later pull can miss the earlier push and clobber it.
 *
 * It is also disabled until the home selection has loaded: before then the
 * pending state is empty, and a full-replace push of it would remove every Uno
 * row from the profile.
 */
export function PushButton({ push, ready, pushing }: Push) {
  return (
    <button
      type="button"
      onClick={push}
      disabled={!ready || pushing}
      // A floor on the width below `lg`. The label swaps to "Pushing…" while a
      // push is in flight, and in a header that no longer wraps the extra width
      // comes out of the profile chip beside it, which re-truncates as it goes.
      // Above `lg` the row is far from full and the button keeps its own width.
      className="btn-tv min-w-[88px] lg:min-w-[auto]"
    >
      <Icon icon={Tv} size={16} />
      <span>
        {pushing ? (
          'Pushing…'
        ) : (
          <>
            Push<span className="hidden lg:inline"> to TV</span>
          </>
        )}
      </span>
    </button>
  )
}

/**
 * The result of the last push. A strip under the header rather than a toast:
 * some outcomes need to stay readable until the user acts on them, and one
 * names a URL they may need to copy. Sticky in every case — it sits inside the
 * header's own sticky band — because the outcome the user sees most often
 * behaving differently from the one they have to act on is its own bug.
 *
 * Success and everything else are separate components because they diverge
 * below `lg`: success collapses to one line there, while a failure keeps every
 * word it has. Splitting them is what keeps that promise legible.
 *
 * No staged progress while in flight — it's one call, and invented stages
 * ("Saving…", "Installing addon…") would claim knowledge the client lacks.
 */
export function PushBanner({ outcome, dismiss }: Push) {
  if (!outcome) return null

  if (outcome.kind === 'success') {
    return <SuccessBanner manifestURL={outcome.manifestURL} dismiss={dismiss} />
  }
  return <ProblemBanner kind={outcome.kind} dismiss={dismiss} />
}

/**
 * The list of changes: every named edit waiting to be pushed, opened from the
 * pending count beside Push. A strip under the header, ahead of the push
 * outcome, matching DESIGN.md's stacking order for the two.
 *
 * Renders nothing while closed or clean — `open` already accounts for both
 * (see `Builder.tsx`'s `PendingIndicator`), so a caller doesn't have to gate
 * this itself.
 */
export function ChangesStrip({
  changes,
  open,
  onHide,
}: {
  changes: HomeChange[]
  open: boolean
  onHide: () => void
}) {
  if (!open || changes.length === 0) return null

  return (
    <div className="bg-raised border-line flex flex-col gap-2 border-b px-4 py-3 lg:px-5">
      <div className="flex items-center justify-between gap-3">
        <p className="text-pending m-0 text-[14px] font-bold">What's not on your TV yet</p>
        <button type="button" onClick={onHide} className="btn-ghost btn-sm shrink-0">
          Hide
        </button>
      </div>
      <ul className="m-0 flex list-none flex-col gap-1.5 p-0">
        {changes.map((change) => (
          <li
            key={change.key}
            className="text-ink flex items-baseline gap-2.5 text-[14px] leading-[1.45]"
          >
            <span aria-hidden="true" className="bg-pending size-1.5 shrink-0 translate-y-[-2px] rounded-full" />
            {change.text}
          </li>
        ))}
      </ul>
    </div>
  )
}

/**
 * Pushed, and below `lg` that is the whole message.
 *
 * The detail line and the URL itself are held back there. Neither is actionable
 * on success: the sentence adds nothing to the headline, and the URL is
 * ellipsised past reading at that width — copying it is the thing the reader
 * actually does with it, so the button stays and the text it labels doesn't.
 * That keeps the banner short on the outcome that happens most.
 */
function SuccessBanner({ manifestURL, dismiss }: { manifestURL?: string; dismiss: () => void }) {
  return (
    <div className="bg-raised border-line flex shrink-0 items-center gap-3 border-b px-4 py-2.5 lg:items-start lg:px-5 lg:py-3">
      <span
        aria-hidden="true"
        className="bg-tv-yellow text-sign-ink grid size-6 shrink-0 place-items-center rounded-full"
      >
        <Icon icon={Check} size={14} />
      </span>

      <div className="flex min-w-0 flex-1 flex-col lg:gap-1">
        <p className="text-tv-yellow m-0 text-[14px] font-bold">{HEADLINE.success}</p>
        <p className="text-dim m-0 hidden text-[13.5px] lg:block">{DETAIL.success}</p>
        {manifestURL && <ManifestURL url={manifestURL} className="hidden lg:flex" />}
      </div>

      {manifestURL && <CopyButton url={manifestURL} label="Copy URL" className="lg:hidden" />}

      <DismissButton onClick={dismiss} />
    </div>
  )
}

/**
 * A push that didn't land, at full text on every screen. These are rare, and
 * the words are the recovery — `undo-failed` in particular is the tallest thing
 * this band can produce, and should be.
 */
function ProblemBanner({
  kind,
  dismiss,
}: {
  kind: Exclude<PushOutcomeKind, 'success'>
  dismiss: () => void
}) {
  return (
    <div className="bg-raised border-line flex shrink-0 items-start gap-3 border-b px-4 py-3 lg:px-5">
      <span
        aria-hidden="true"
        className="bg-danger text-danger-ink grid size-6 shrink-0 place-items-center rounded-full"
      >
        <Icon icon={TriangleAlert} size={14} />
      </span>

      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <p className="text-danger m-0 text-[14px] font-bold">{HEADLINE[kind]}</p>
        <p className="text-dim m-0 text-[13.5px]">{DETAIL[kind]}</p>
      </div>

      <DismissButton onClick={dismiss} />
    </div>
  )
}

function DismissButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label="Dismiss"
      className="tap text-dim hover:text-ink hover:bg-raised-hi grid h-8 w-8 shrink-0 place-items-center rounded-full transition-colors"
    >
      <Icon icon={X} size={16} />
    </button>
  )
}

const HEADLINE: Record<PushOutcomeKind, string> = {
  success: 'Pushed.',
  failed: 'Push failed — nothing changed.',
  'undo-failed': "Push failed, and we couldn't fully undo it.",
  unknown: "Couldn't confirm what happened.",
}

const DETAIL: Record<PushOutcomeKind, string> = {
  success: 'Your home screen is saved and synced to Nuvio.',
  failed: 'Your edits are still here. Try again.',
  'undo-failed':
    "Nuvio may be out of step with what's saved here. Push again to put them back in line.",
  unknown: "We couldn't tell whether this push worked. Reload to see what's live.",
}

type PushOutcomeKind = NonNullable<Push['outcome']>['kind']

function CopyButton({
  url,
  label,
  className = '',
}: {
  url: string
  label: string
  className?: string
}) {
  const { copied, copy } = useCopy(url)

  return (
    <button type="button" onClick={copy} className={`btn-ghost btn-sm shrink-0 ${className}`}>
      <Icon icon={copied ? Check : Copy} size={14} />
      {copied ? 'Copied' : label}
    </button>
  )
}

/**
 * Shown on success, because a first push is followed by installing this URL in
 * Nuvio. It is a capability URL for a public, read-only surface: fine to
 * display, but it must not be logged or put anywhere shareable.
 */
function ManifestURL({ url, className = '' }: { url: string; className?: string }) {
  return (
    <div className={`flex min-w-0 items-center gap-2 ${className}`}>
      <code className="type-data text-dim min-w-0 truncate text-[13px]">{url}</code>
      <CopyButton url={url} label="Copy" />
    </div>
  )
}

/**
 * The addon URL, available from the moment a profile is picked rather than only
 * after a first successful push — `POST /api/profiles/select` returns it, built
 * server-side from `SITE_BASE_URL`. Don't rebuild it from
 * `window.location.origin`: Vite serves on its own port in dev, so that would
 * be wrong exactly where this gets tested.
 *
 * Above `lg` only. Below it the same action is an item in `ProfileMenu`, which
 * is where the header's room for a fourth control went.
 */
export function AddonURLButton({ url, className = '' }: { url: string; className?: string }) {
  const { copied, copy } = useCopy(url)

  return (
    <button
      type="button"
      onClick={copy}
      title={`Install this in Nuvio to get your home screen:\n${url}`}
      className={`btn-ghost ${className}`}
    >
      <Icon icon={copied ? Check : Link} size={16} />
      {copied ? 'Copied' : 'Addon URL'}
    </button>
  )
}
