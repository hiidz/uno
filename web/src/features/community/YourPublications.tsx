import { useMemo, useState } from 'react'
import { ChevronRight, Search } from 'lucide-react'
import { Button } from '@/components/Button'
import { Icon } from '@/components/Icon'
import { Modal, ModalBody, ModalFooter, ModalHeader } from '@/components/Modal'
import { SharingStickers } from '@/features/sharing/SharingStickers'
import {
  COLLECTION_KIND,
  countsLine,
  isPublished,
  kindSticker,
  ownName,
  publicationCounts,
  railStickers,
  stickerClass,
  stickerWords,
  type OwnRow,
  type SharingSticker,
} from '@/features/sharing/sharingState'

/**
 * The way into the profile's own rows, heading Community above its controls,
 * which filter everyone else's: a card that navigates, counting how many are
 * published and how many are not, with a filled To publish sticker while
 * edits wait to go out. `choices` is empty until the library loads.
 */
export function YourPublicationsCard({
  choices,
  loaded,
  onOpen,
}: {
  choices: OwnRow[]
  loaded: boolean
  onOpen: () => void
}) {
  const counts = publicationCounts(choices)
  const waiting = counts.toPublish > 0 ? `${counts.toPublish} to publish` : ''
  return (
    // drift-ok: a card that navigates, drawn as the Community row's family
    <button
      type="button"
      onClick={onOpen}
      className="bg-raised hover:bg-raised-hi active:bg-raised-hi grid w-full cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-center gap-3 rounded-[14px] py-3.5 pr-4 pl-[18px] text-left transition-colors focus-visible:outline-2 focus-visible:outline-offset-2"
    >
      <span className="flex min-w-0 flex-col gap-1.5">
        <span className="text-title">Your publications</span>
        <span className="flex min-h-5 flex-wrap items-center gap-2">
          {waiting && <span className={stickerClass({ hue: 'community', fill: true })}>{waiting}</span>}
          {loaded && <span className="text-dim text-meta type-data">{countsLine(counts)}</span>}
        </span>
      </span>
      <Icon icon={ChevronRight} size={20} className="text-dim" />
    </button>
  )
}

/** A row's stickers under Your publications: its kind, then its Community
 *  sticker. */
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
 * Your publications: every catalog and collection the profile can publish
 * (`publishChoices`), under Published and Not published yet. A published row
 * opens its own page, where its changes are published and it is unpublished;
 * a private one publishes, through the publish dialog.
 */
export function YourPublicationsDialog({
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
    <Modal open={open} onClose={onClose} labelledBy="your-publications-title" width="520px">
      <ModalHeader>
        <h2 id="your-publications-title" className="type-display text-display m-0">
          Your publications
        </h2>
      </ModalHeader>
      {open && <DialogBody choices={choices} onPublish={onPublish} onOpenPage={onOpenPage} />}
      <ModalFooter>
        <Button kind="ghost" onClick={onClose}>
          Close
        </Button>
      </ModalFooter>
    </Modal>
  )
}

function DialogBody({
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
        <p className="text-dimmer text-meta m-0">Nothing of yours to publish yet. Build a catalog in Configure first.</p>
      </ModalBody>
    )
  }
  const published = shown.filter((own) => isPublished(own.row))
  const unpublished = shown.filter((own) => !isPublished(own.row))
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
          <>
            <Group label="Published" rows={published} onPublish={onPublish} onOpenPage={onOpenPage} />
            <Group label="Not published yet" rows={unpublished} onPublish={onPublish} onOpenPage={onOpenPage} />
          </>
        )}
      </div>
    </ModalBody>
  )
}

/** One group under its label; nothing while it is empty. */
function Group({
  label,
  rows,
  onPublish,
  onOpenPage,
}: {
  label: string
  rows: OwnRow[]
  onPublish: (own: OwnRow) => void
  onOpenPage: (own: OwnRow) => void
}) {
  if (rows.length === 0) return null
  return (
    <section aria-label={label} className="flex flex-col gap-1">
      <h3 className="type-label text-dim m-0 mt-1">{label}</h3>
      <ul className="-mx-3 m-0 flex list-none flex-col gap-1 p-0">
        {rows.map((own) => (
          <li key={own.row.id}>
            <PickRow own={own} onPublish={onPublish} onOpenPage={onOpenPage} />
          </li>
        ))}
      </ul>
    </section>
  )
}

/** One row: its name over its stickers, and at its end what choosing it
 *  does — a chevron to the page of a published row, Publish… for a private
 *  one. */
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
