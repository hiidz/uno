import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { acknowledgeRelease, fetchReleased, queryKeys, type ReleasedCopy } from '@/api'
import { Modal, ModalBody, ModalFooter, ModalHeader } from '@/components/Modal'
import { errorText } from './sharingState'

/**
 * Tells the profile, once each, about a row its publisher took out of
 * Community, which made it the profile's own: one dialog per released row,
 * oldest first, read with the profile and on every refocus. However it is
 * dismissed — its button, Escape, the scrim — dismissing acknowledges the
 * release, and the server's answer, the rows still unacknowledged, opens the
 * next. A failed acknowledgement rereads the list, which drops a row deleted
 * elsewhere; one still listed keeps the dialog open and says why.
 */
export function ReleasedDialog({ profileIndex }: { profileIndex: number }) {
  const queryClient = useQueryClient()
  const queryKey = queryKeys.released(profileIndex)
  const { data } = useQuery({ queryKey, queryFn: () => fetchReleased(profileIndex) })
  const acknowledge = useMutation({
    mutationFn: (row: ReleasedCopy) => acknowledgeRelease(profileIndex, row.kind, row.id),
    onMutate: () => queryClient.cancelQueries({ queryKey }),
    onSuccess: (remaining) => queryClient.setQueryData(queryKey, remaining),
    onError: () => queryClient.invalidateQueries({ queryKey }),
  })
  const row = data?.[0]
  if (!row) return null

  function dismiss() {
    if (row && !acknowledge.isPending) acknowledge.mutate(row)
  }

  const error = errorText(acknowledge.error)
  return (
    <Modal open onClose={dismiss} labelledBy="released-title" width="440px">
      <ModalHeader>
        <h2 id="released-title" className="type-display m-0 text-[18px]">
          Removed from Community
        </h2>
      </ModalHeader>
      <ModalBody>
        <p className="text-dim m-0 text-[14.5px] leading-relaxed">
          Its publisher removed “{row.name}” from Community. It’s now yours to edit.
        </p>
        {error && <p role="alert" className="callout-danger type-data mt-4">{error}</p>}
      </ModalBody>
      <ModalFooter tone="community">
        <button type="button" onClick={dismiss} disabled={acknowledge.isPending} className="btn-primary">
          Got it
        </button>
      </ModalFooter>
    </Modal>
  )
}
