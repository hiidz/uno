import { useId, type ReactNode } from 'react'
import type { Catalog } from '@/api'
import { Modal, ModalBody, ModalFooter, ModalHeader } from '@/components/Modal'
import type { GenreLookups } from '@/features/library/useLibrary'
import { pluralCount } from '@/lib/plural'
import { CatalogList, type CatalogListItem } from './CatalogList'
import { catalogItem } from './listing'
import { FROM_COMMUNITY, kindSticker } from './sharingState'

/** What a publish publishes: one catalog, or a collection's folders with the
 *  catalogs it holds (`own`) and the library catalogs it uses (`library`). */
export type PublishSubject =
  | { kind: 'catalog'; name: string; catalog: Catalog }
  | { kind: 'collection'; name: string; folderCount: number; own: Catalog[]; library: Catalog[] }

/** The dialog's words for a first publish (or again after Unpublish), and
 *  for publishing changes to something already published. */
const FIRST_WORDS = {
  title: (name: string) => `Publish “${name}”?`,
  line: 'Anyone on Uno can find it in Community and add it. Your later edits stay private until you publish an update.',
  confirm: 'Publish',
}
const UPDATE_WORDS = {
  title: (name: string) => `Publish your changes to “${name}”?`,
  line: 'People who added it are offered this version. Until then they keep the one they have.',
  confirm: 'Publish update',
}

/** A catalog of a collection's group as a list row, marked From Community
 *  when it was added from there. */
function groupItem(catalog: Catalog, genres: GenreLookups): CatalogListItem {
  const item = catalogItem(catalog, genres)
  if (!catalog.subscription) return item
  return { ...item, sticker: FROM_COMMUNITY }
}

function CatalogGroup({ title, catalogs, genres }: { title: string; catalogs: Catalog[]; genres: GenreLookups }) {
  const items = catalogs.map((catalog) => groupItem(catalog, genres))
  return (
    <div>
      <h3 className="type-label m-0">{title}</h3>
      <CatalogList items={items} />
    </div>
  )
}

/** The library catalogs a collection uses, published as they stand; nothing
 *  when it uses none. */
function LibraryGroup({ catalogs, genres }: { catalogs: Catalog[]; genres: GenreLookups }) {
  if (catalogs.length === 0) return null
  return <CatalogGroup title="From your library, published as they are now" catalogs={catalogs} genres={genres} />
}

/** The server's refusal, in the dialog. */
function DialogError({ error }: { error: string | null }) {
  if (!error) return null
  return (
    <p role="alert" className="callout-danger type-data m-0">
      {error}
    </p>
  )
}

interface UnpublishLinkProps {
  published: boolean
  onUnpublish: () => void
}

/** The red text Unpublish at the footer's start, for a published row; it
 *  opens the Unpublish confirmation. */
function UnpublishLink({ published, onUnpublish }: UnpublishLinkProps) {
  if (!published) return null
  return (
    <button type="button" onClick={onUnpublish} className="btn-danger-text mr-auto">
      Unpublish
    </button>
  )
}

/** Everything the publication will hold: one catalog, or a collection's own
 *  catalogs and then the library catalogs it uses. */
function PublishContents({ subject, genres }: { subject: PublishSubject; genres: GenreLookups }) {
  if (subject.kind === 'catalog') {
    return <CatalogList items={[catalogItem(subject.catalog, genres, kindSticker(subject.catalog.type))]} />
  }
  return (
    <>
      <CatalogGroup
        title={`${pluralCount(subject.folderCount, 'folder')}, ${pluralCount(subject.own.length, 'catalog')} of its own`}
        catalogs={subject.own}
        genres={genres}
      />
      <LibraryGroup catalogs={subject.library} genres={genres} />
    </>
  )
}

interface PublishDialogProps {
  open: boolean
  subject: PublishSubject | null
  /** Publishing changes to something already published, rather than
   *  publishing it. */
  update: boolean
  /** What those changes are (`SinceLastPublished`), which shows only when
   *  `update` says there are some. */
  since: ReactNode
  genres: GenreLookups
  pending: boolean
  error: string | null
  onConfirm: () => void
  onClose: () => void
  /** Unpublishes instead, offered while publishing changes: the row is
   *  published, so taking it down is the other step it has. */
  onUnpublish: () => void
}

/**
 * Asks before publishing a row, or publishing its update, and lists
 * everything the publication will hold, each catalog with its recipe line. A
 * collection's library catalogs sit under their own heading: they are
 * published as they are now, which someone reading the collection's name
 * alone might not expect. A catalog added from Community carries the From
 * Community sticker, so it is clear it is someone else's catalog being
 * published as it stands. Publishing makes no copy for anyone; people add it
 * from Community. When it publishes changes, `since` lists what they are,
 * above what the publication will hold.
 */
export function PublishDialog({
  open,
  subject,
  update,
  since,
  genres,
  pending,
  error,
  onConfirm,
  onClose,
  onUnpublish,
}: PublishDialogProps) {
  const titleID = useId()
  if (!subject) return null
  const words = update ? UPDATE_WORDS : FIRST_WORDS

  return (
    <Modal open={open} onClose={onClose} labelledBy={titleID} width="540px">
      <ModalHeader>
        <h2 id={titleID} className="type-display m-0 text-[21px] leading-[28px]">
          {words.title(subject.name)}
        </h2>
      </ModalHeader>
      <ModalBody>
        <div className="flex flex-col gap-4 text-[14.5px] leading-relaxed">
          <p className="text-dim m-0">{words.line}</p>
          {since}
          <PublishContents subject={subject} genres={genres} />
          <DialogError error={error} />
        </div>
      </ModalBody>
      <ModalFooter tone="community">
        <UnpublishLink published={update} onUnpublish={onUnpublish} />
        <button type="button" onClick={onClose} className="btn-ghost">
          Cancel
        </button>
        <button type="button" onClick={onConfirm} disabled={pending} className="btn-primary">
          {pending ? 'Publishing…' : words.confirm}
        </button>
      </ModalFooter>
    </Modal>
  )
}
