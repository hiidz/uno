import { Button } from '@/components/Button'

interface ProfilesRetryProps {
  /** The profile list failed to load. */
  failed: boolean
  /** The failure is this server refusing the account, which asking again
   *  doesn't change. */
  refused: boolean
  onRetry(): unknown
}

/** Retry for the profile picker's list once its own retries are spent, which
 *  otherwise only a window refocus or a reload would fetch again. */
export function ProfilesRetry({ failed, refused, onRetry }: ProfilesRetryProps) {
  if (!failed || refused) return null
  return (
    <Button kind="ghost" onClick={() => void onRetry()} className="mb-4">
      Retry
    </Button>
  )
}
