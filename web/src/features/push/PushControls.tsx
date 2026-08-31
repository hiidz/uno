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
 */
export function PushButton({ push, pushing }: Push) {
  return (
    <button
      type="button"
      onClick={push}
      disabled={pushing}
      // A floor on the width below `lg`. The label swaps to "Pushing…" while a
      // push is in flight, and in a header that no longer wraps the extra width
      // comes out of the profile chip beside it, which re-truncates as it goes.
      // Above `lg` the row is far from full and the button keeps its own width.
      className="btn-primary min-w-[78px] lg:min-w-[auto]"
    >
      {pushing ? 'Pushing…' : 'Push'}
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
 * Pushed, and below `lg` that is the whole message.
 *
 * The detail line and the URL itself are held back there. Neither is actionable
 * on success: the sentence adds nothing to the headline, and the URL is
 * ellipsised past reading at that width — copying it is the thing the reader
 * actually does with it, so the button stays and the text it labels doesn't.
 * That is 100px of frozen chrome down to 52px, on the outcome that happens
 * most.
 */
function SuccessBanner({ manifestURL, dismiss }: { manifestURL?: string; dismiss: () => void }) {
  return (
    <div className="bg-raised border-line flex shrink-0 items-center gap-3 border-b px-4 py-2 lg:items-start lg:px-5 lg:py-2.5">
      <span aria-hidden="true" className="text-movie type-data text-[11px] lg:mt-[1px]">
        ✓
      </span>

      <div className="flex min-w-0 flex-1 flex-col lg:gap-1.5">
        <p className="type-data text-movie m-0 text-[11px]">{HEADLINE.success}</p>
        <p className="type-data text-dim m-0 hidden text-[10.5px] lg:block">{DETAIL.success}</p>
        {manifestURL && <ManifestURL url={manifestURL} className="hidden lg:flex" />}
      </div>

      {manifestURL && <CopyButton url={manifestURL} label="copy url" className="lg:hidden" />}

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
    <div className="bg-raised border-line flex shrink-0 items-start gap-3 border-b px-4 py-2.5 lg:px-5">
      <span aria-hidden="true" className="text-danger type-data mt-[1px] text-[11px]">
        !
      </span>

      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <p className="type-data text-danger m-0 text-[11px]">{HEADLINE[kind]}</p>
        <p className="type-data text-dim m-0 text-[10.5px]">{DETAIL[kind]}</p>
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
      className="tap text-dimmer hover:text-ink grid h-5 w-5 shrink-0 place-items-center leading-none transition-colors"
    >
      ×
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
    <button type="button" onClick={copy} className={`btn-ghost shrink-0 ${className}`}>
      {copied ? 'copied' : label}
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
      <code className="type-data text-dimmer min-w-0 truncate text-[10.5px]">{url}</code>
      <CopyButton url={url} label="copy" />
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
      {copied ? 'copied' : 'addon url'}
    </button>
  )
}
