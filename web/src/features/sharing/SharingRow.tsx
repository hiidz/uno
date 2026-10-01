import type { PublicationState } from '@/api'
import { FieldError } from '@/components/fields'
import { ownSharing, sharingNote, type OwnSharing } from './sharingState'

const WORDS: Record<OwnSharing, string> = {
  private: 'Private',
  live: 'Published',
  changed: 'Published. Your changes since aren’t published yet.',
  unpublished: 'Unpublished. People who added it keep it.',
}

const PUBLISH_LABEL: Record<OwnSharing, string> = {
  private: 'Publish…',
  live: 'Publish…',
  changed: 'Publish update…',
  unpublished: 'Publish again…',
}

/**
 * An own listed row's Community setting in its editor: where the row stands
 * with Community in words, then Publish… (which opens the publish dialog
 * listing what is published), Publish update… once the saved row has changed
 * since, and Unpublish. Publishing publishes the saved row, so it waits while
 * the form has unsaved changes.
 */
export function SharingRow({
  publication,
  dirty,
  onPublish,
  onUnpublish,
}: {
  publication: PublicationState | null
  dirty: boolean
  onPublish: () => void
  onUnpublish: () => void
}) {
  const state = ownSharing(publication)
  const published = state === 'live' || state === 'changed'
  const note = sharingNote(state, dirty)

  return (
    <div className="setting">
      <span className="setting-label type-label">Community</span>
      <div className="setting-value flex flex-col items-start gap-2">
        <span className="type-data text-[15px]">{WORDS[state]}</span>
        <div className="flex flex-wrap gap-2">
          {state !== 'live' && (
            <button type="button" className="btn-secondary btn-sm" disabled={dirty} onClick={onPublish}>
              {PUBLISH_LABEL[state]}
            </button>
          )}
          {published && (
            <button type="button" className="btn-ghost btn-sm" onClick={onUnpublish}>
              Unpublish
            </button>
          )}
        </div>
        {note && <FieldError tone="caution">{note}</FieldError>}
      </div>
    </div>
  )
}
