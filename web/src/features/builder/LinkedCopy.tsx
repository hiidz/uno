import { ConfirmDialog } from '@/components/ConfirmDialog'

type LinkedNoun = 'catalog' | 'collection'

/** The first row of an editor open on a linked copy — a library catalog or a
 *  collection taken from Community and not edited since. A collection's banner
 *  also covers the catalogs edited inside it, which have no link of their own. */
export function LinkedBanner({ noun }: { noun: LinkedNoun }) {
  return (
    <div className="setting" role="note">
      <span className="setting-label type-label">Linked</span>
      <p className="setting-value m-0 text-[14px] leading-[20px]">
        Linked to a community {noun}. Saving changes unlinks it, and you'll stop getting the
        owner's updates.
      </p>
    </div>
  )
}

/** Asked before a save that would unlink a linked copy (`changesContent`).
 *  Keep editing leaves the form as it is. */
export function ConfirmUnlink({
  open,
  noun,
  name,
  onConfirm,
  onCancel,
}: {
  open: boolean
  noun: LinkedNoun
  name: string
  onConfirm: () => void
  onCancel: () => void
}) {
  return (
    <ConfirmDialog
      open={open}
      title="Save and unlink?"
      body={
        <>
          Saving your changes to <strong className="text-ink">{name}</strong> unlinks it from the
          community {noun} it was taken from, and you'll stop getting the owner's updates.
        </>
      }
      confirmLabel="Save and unlink"
      cancelLabel="Keep editing"
      onConfirm={onConfirm}
      onCancel={onCancel}
    />
  )
}
