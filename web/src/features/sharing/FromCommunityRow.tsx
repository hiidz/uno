import { Copy, RefreshCw, Unlink } from 'lucide-react'
import type { SubscriptionState } from '@/api'
import { FieldError, InfoTip } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { fromWords } from './sharingState'

/**
 * A copy's setting in its editor, where an own row has its Sharing row:
 * where it came from and what can be done with it. Update…, while its owner
 * has published one, opens the publication's page in Community; Detach makes
 * the copy the profile's own without changing it, and waits while the form
 * has unsaved changes, since saving them does the same; Duplicate makes a
 * separate own copy, which is how to share a version of it. Its buttons are
 * quiet: Save stays the editor's one primary.
 */
export function FromCommunityRow({
  subscription,
  dirty,
  onUpdate,
  onDetach,
  onDuplicate,
}: {
  subscription: SubscriptionState
  dirty: boolean
  onUpdate: () => void
  onDetach: () => void
  onDuplicate?: () => void
}) {
  return (
    <div className="setting">
      <span className="setting-label type-label">From Community</span>
      <div className="setting-value flex flex-col items-start gap-2">
        <span className="text-[14.5px] leading-[1.45]">{fromWords(subscription)}</span>
        <div className="flex flex-wrap items-center gap-2">
          {subscription.update_available && (
            <button type="button" className="btn-secondary btn-sm" onClick={onUpdate}>
              <Icon icon={RefreshCw} size={14} />
              Update…
            </button>
          )}
          <button type="button" className="btn-secondary btn-sm" disabled={dirty} onClick={onDetach}>
            <Icon icon={Unlink} size={14} />
            Detach
          </button>
          {onDuplicate && (
            <button type="button" className="btn-secondary btn-sm" onClick={onDuplicate}>
              <Icon icon={Copy} size={14} />
              Duplicate
            </button>
          )}
          <InfoTip
            label="Detach and Duplicate"
            text="Saving a change, or Detach, makes this copy yours to share, and it stops getting updates. Duplicate makes a separate copy that's yours, and this one keeps following its owner."
          />
        </div>
        {dirty && <FieldError tone="caution">Saving your changes makes this copy yours.</FieldError>}
      </div>
    </div>
  )
}
