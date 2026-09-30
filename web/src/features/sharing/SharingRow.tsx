import type { PublicationState } from '@/api'
import { FieldError } from '@/components/fields'
import { ownSharing, sharingNote, type OwnSharing } from './sharingState'

const WORDS: Record<OwnSharing, string> = {
  private: 'Private',
  live: 'Shared',
  changed: 'Shared. Your changes since aren’t shared yet.',
  withdrawn: 'Not shared any more. Copies people took stay theirs.',
}

const SHARE_LABEL: Record<OwnSharing, string> = {
  private: 'Share…',
  live: 'Share…',
  changed: 'Publish update…',
  withdrawn: 'Share again…',
}

/**
 * An own listed row's Sharing setting in its editor: where the row stands
 * with Community in words, then Share… (which opens the publish dialog
 * listing what is shared), Publish update… once the saved row has changed
 * since, and Stop sharing. Sharing publishes the saved
 * row, so it waits while the form has unsaved changes. `blocked` is why this
 * row can't be shared at all — a collection using a catalog taken from
 * Community — shown under a greyed Share….
 */
export function SharingRow({
  publication,
  dirty,
  blocked,
  onShare,
  onStop,
}: {
  publication: PublicationState | null
  dirty: boolean
  blocked: string | null
  onShare: () => void
  onStop: () => void
}) {
  const state = ownSharing(publication)
  const shared = state === 'live' || state === 'changed'
  const waiting = dirty || blocked !== null
  const note = sharingNote(state, dirty, blocked)

  return (
    <div className="setting">
      <span className="setting-label type-label">Sharing</span>
      <div className="setting-value flex flex-col items-start gap-2">
        <span className="type-data text-[15px]">{WORDS[state]}</span>
        <div className="flex flex-wrap gap-2">
          {state !== 'live' && (
            <button type="button" className="btn-secondary btn-sm" disabled={waiting} onClick={onShare}>
              {SHARE_LABEL[state]}
            </button>
          )}
          {shared && (
            <button type="button" className="btn-ghost btn-sm" onClick={onStop}>
              Stop sharing
            </button>
          )}
        </div>
        {note && <FieldError tone="caution">{note}</FieldError>}
      </div>
    </div>
  )
}
