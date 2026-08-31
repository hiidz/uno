import { useState } from 'react'
import type { Push } from './usePush'

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
    <button type="button" onClick={push} disabled={pushing} className="btn-primary">
      {pushing ? 'Pushing…' : 'Push'}
    </button>
  )
}

/**
 * The result of the last push. A strip under the header rather than a toast:
 * some outcomes need to stay readable until the user acts on them, and one
 * names a URL they may need to copy.
 *
 * No staged progress while in flight — it's one call, and invented stages
 * ("Saving…", "Installing addon…") would claim knowledge the client lacks.
 */
export function PushBanner({ outcome, dismiss }: Push) {
  if (!outcome) return null

  const tone = outcome.kind === 'success' ? 'text-movie' : 'text-danger'

  return (
    <div className="bg-raised border-line flex shrink-0 items-start gap-3 border-b px-5 py-2.5">
      <span aria-hidden="true" className={`${tone} type-data mt-[1px] text-[11px]`}>
        {outcome.kind === 'success' ? '✓' : '!'}
      </span>

      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <p className={`type-data m-0 text-[11px] ${tone}`}>{HEADLINE[outcome.kind]}</p>
        <p className="type-data text-dim m-0 text-[10.5px]">{DETAIL[outcome.kind]}</p>
        {outcome.kind === 'success' && outcome.manifestURL && (
          <ManifestURL url={outcome.manifestURL} />
        )}
      </div>

      <button
        type="button"
        onClick={dismiss}
        aria-label="Dismiss"
        className="tap text-dimmer hover:text-ink grid h-5 w-5 shrink-0 place-items-center leading-none transition-colors"
      >
        ×
      </button>
    </div>
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

/**
 * Copy-to-clipboard, returning the label to show. Clipboard access can fail
 * outright (insecure origin, denied permission), so the "copied" confirmation
 * is only shown once the write actually resolves.
 */
function useCopy(url: string): { copied: boolean; copy: () => void } {
  const [copied, setCopied] = useState(false)

  function copy() {
    navigator.clipboard
      .writeText(url)
      .then(() => {
        setCopied(true)
        setTimeout(() => setCopied(false), 1600)
      })
      .catch(() => {
        // Nothing to recover from — just don't claim it copied.
      })
  }

  return { copied, copy }
}

/**
 * Shown on success, because a first push is followed by installing this URL in
 * Nuvio. It is a capability URL for a public, read-only surface: fine to
 * display, but it must not be logged or put anywhere shareable.
 */
function ManifestURL({ url }: { url: string }) {
  const { copied, copy } = useCopy(url)

  return (
    <div className="flex min-w-0 items-center gap-2">
      <code className="type-data text-dimmer min-w-0 truncate text-[10.5px]">{url}</code>
      <button type="button" onClick={copy} className="btn-ghost shrink-0">
        {copied ? 'copied' : 'copy'}
      </button>
    </div>
  )
}

/**
 * The addon URL, available from the moment a profile is picked rather than only
 * after a first successful push — `POST /api/profiles/select` returns it, built
 * server-side from `SITE_BASE_URL`. Don't rebuild it from
 * `window.location.origin`: Vite serves on its own port in dev, so that would
 * be wrong exactly where this gets tested.
 */
export function AddonURLButton({ url }: { url: string }) {
  const { copied, copy } = useCopy(url)

  return (
    <button
      type="button"
      onClick={copy}
      title={`Install this in Nuvio to get your home screen:\n${url}`}
      className="btn-ghost"
    >
      {copied ? 'copied' : 'addon url'}
    </button>
  )
}
