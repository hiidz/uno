import { useMemo, useState } from 'react'
import { ChevronRight, Search } from 'lucide-react'
import { Button } from '@/components/Button'
import { Icon } from '@/components/Icon'
import { Modal, ModalBody, ModalFooter, ModalHeader } from '@/components/Modal'
import { SharingStickers } from '@/features/sharing/SharingStickers'
import {
  COLLECTION_KIND,
  isPublished,
  kindSticker,
  ownName,
  railStickers,
  stickerWords,
  type OwnRow,
  type SharingSticker,
} from '@/features/sharing/sharingState'

/** A picker row's stickers: its kind, then its Community sticker. */
function pickStickers(own: OwnRow): SharingSticker[] {
  const kind = own.kind === 'catalog' ? kindSticker(own.row.type) : COLLECTION_KIND
  return [kind, ...railStickers(own.row)]
}

/** Whether every word of `q` is in the row's name. */
function matches(own: OwnRow, q: string): boolean {
  const name = ownName(own).toLowerCase()
  return q
    .toLowerCase()
    .split(/\s+/)
    .every((word) => name.includes(word))
}

/**
 * The way into Community for the profile's own rows: every catalog and
 * collection it can publish (`publishChoices`), each with its kind and where
 * it stands with Community. Choosing a private row publishes it, through the
 * publish dialog; choosing a published one opens its page, where its changes
 * are published and it is unpublished.
 */
export function PublishPicker({
  open,
  choices,
  onPublish,
  onOpenPage,
  onClose,
}: {
  open: boolean
  choices: OwnRow[]
  onPublish: (own: OwnRow) => void
  onOpenPage: (own: OwnRow) => void
  onClose: () => void
}) {
  return (
    <Modal open={open} onClose={onClose} labelledBy="publish-picker-title" width="520px" shape="sheet">
      <ModalHeader>
        <h2 id="publish-picker-title" className="type-display text-display m-0">
          Publish to Community
        </h2>
      </ModalHeader>
      {open && <PickerBody choices={choices} onPublish={onPublish} onOpenPage={onOpenPage} />}
      <ModalFooter>
        <Button kind="ghost" onClick={onClose}>
          Close
        </Button>
      </ModalFooter>
    </Modal>
  )
}

function PickerBody({
  choices,
  onPublish,
  onOpenPage,
}: {
  choices: OwnRow[]
  onPublish: (own: OwnRow) => void
  onOpenPage: (own: OwnRow) => void
}) {
  const [q, setQ] = useState('')
  const shown = useMemo(() => choices.filter((own) => matches(own, q.trim())), [choices, q])
  if (choices.length === 0) {
    return (
      <ModalBody>
        <p className="text-dimmer text-meta m-0">Nothing of yours to publish yet. Build a catalog in the Workspace first.</p>
      </ModalBody>
    )
  }
  return (
    <ModalBody>
      <div className="flex flex-col gap-3">
        <div className="relative">
          <Icon icon={Search} size={16} className="text-dimmer pointer-events-none absolute top-1/2 left-4 -translate-y-1/2" />
          <input
            type="search"
            value={q}
            onChange={(event) => setQ(event.target.value)}
            placeholder="Search yours"
            aria-label="Search your catalogs and collections"
            className="field text-body-sm pointer-coarse:text-field h-10 w-full rounded-full pl-10 pointer-coarse:h-11"
          />
        </div>
        {shown.length === 0 ? (
          <p className="text-dimmer text-meta m-0">Nothing of yours matches this search.</p>
        ) : (
          <ul className="-mx-3 m-0 flex list-none flex-col gap-1 p-0">
            {shown.map((own) => (
              <li key={own.row.id}>
                <PickRow own={own} onPublish={onPublish} onOpenPage={onOpenPage} />
              </li>
            ))}
          </ul>
        )}
      </div>
    </ModalBody>
  )
}

/** One row of the picker: its name over its stickers, and at its end what
 *  choosing it does — Publish… for a private row, a chevron to the page of a
 *  published one. */
function PickRow({
  own,
  onPublish,
  onOpenPage,
}: {
  own: OwnRow
  onPublish: (own: OwnRow) => void
  onOpenPage: (own: OwnRow) => void
}) {
  const published = isPublished(own.row)
  const stickers = pickStickers(own)
  return (
    // drift-ok: a whole-row choice, drawn as the Library row's family
    <button
      type="button"
      onClick={() => (published ? onOpenPage(own) : onPublish(own))}
      className="hover:bg-raised-hi active:bg-line grid w-full cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 rounded-xl px-3 py-2.5 text-left transition-colors focus-visible:outline-2 focus-visible:outline-offset-[-2px]"
    >
      <span className="flex min-w-0 flex-col gap-1">
        <span className="text-body font-semibold [overflow-wrap:anywhere]">
          {ownName(own)}
          <span className="sr-only">{stickerWords(stickers)}</span>
        </span>
        <span aria-hidden="true" className="flex flex-wrap gap-1.5">
          <SharingStickers stickers={stickers} />
        </span>
      </span>
      {published ? (
        <Icon icon={ChevronRight} size={18} className="text-dim" />
      ) : (
        <span className="text-dim text-body-sm font-semibold">Publish…</span>
      )}
    </button>
  )
}
