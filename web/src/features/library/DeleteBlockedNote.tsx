/**
 * Why a row's Delete is disabled, in words beside it: Nuvio may still hold
 * the row (`deleteBlockers.ts`). Hidden while nothing holds it, so a caller
 * renders it unconditionally and it takes no room until it has something to
 * say.
 */
export function DeleteBlockedNote({ reason, className = '' }: { reason: string | null; className?: string }) {
  return (
    <p hidden={reason === null} className={`text-dim m-0 text-[12.5px] leading-snug ${className}`}>
      Can’t delete yet. {reason}
    </p>
  )
}
