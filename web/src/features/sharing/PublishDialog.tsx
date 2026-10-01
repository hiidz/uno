import { useId } from 'react'
import type { Catalog } from '@/api'
import { Modal, ModalBody, ModalFooter, ModalHeader } from '@/components/Modal'
import { typeLabel } from '@/features/library/recipe'
import type { GenreLookups } from '@/features/library/useLibrary'
import { pluralCount } from '@/lib/plural'
import { CatalogList, type CatalogListItem } from './CatalogList'
import { catalogItem } from './listing'
import { FROM_COMMUNITY } from './sharingState'

/** What a publish shares: one catalog, or a collection's folders with the
 *  catalogs it holds (`own`) and the library catalogs it uses (`library`). */
export type PublishSubject =
  | { kind: 'catalog'; name: string; catalog: Catalog }
  | { kind: 'collection'; name: string; folderCount: number; own: Catalog[]; library: Catalog[] }

/**
 * Asks before sharing a row, or publishing its update, and lists everything
 * the publication will hold, each catalog with its recipe line. A collection's
 * library catalogs sit under their own heading: they are shared as they are
 * now, which someone reading the collection's name alone might not expect. A
 * catalog added from Community carries the From Community sticker, so it is
 * clear it is someone else's catalog being shared as it stands.
 * Sharing makes no copy for anyone; people take one from Community.
 */
export function PublishDialog({
  open,
  subject,
  update,
  genres,
  pending,
  error,
  onConfirm,
  onClose,
}: {
  open: boolean
  subject: PublishSubject | null
  /** Publishing changes to something already shared, rather than sharing it. */
  update: boolean
  genres: GenreLookups
  pending: boolean
  error: string | null
  onConfirm: () => void
  onClose: () => void
}) {
  const titleID = useId()
  if (!subject) return null
  const confirm = update ? 'Publish update' : 'Share'

  return (
    <Modal open={open} onClose={onClose} labelledBy={titleID} width="540px">
      <ModalHeader>
        <h2 id={titleID} className="type-display m-0 text-[21px] leading-[28px]">
          {update ? `Publish your changes to “${subject.name}”?` : `Share “${subject.name}”?`}
        </h2>
      </ModalHeader>
      <ModalBody>
        <div className="flex flex-col gap-4 text-[14.5px] leading-relaxed">
          <p className="text-dim m-0">
            {update
              ? 'People who took a copy are offered this version. Until then they keep the one they have.'
              : 'Anyone on Uno can find it in Community and take a copy. Your later edits stay private until you publish an update.'}
          </p>
          {subject.kind === 'catalog' ? (
            <CatalogList items={[catalogItem(subject.catalog, genres, typeLabel(subject.catalog.type))]} />
          ) : (
            <>
              <CatalogGroup
                title={`${pluralCount(subject.folderCount, 'folder')}, ${pluralCount(subject.own.length, 'catalog')} of its own`}
                catalogs={subject.own}
                genres={genres}
              />
              {subject.library.length > 0 && (
                <CatalogGroup
                  title="From your library, shared as they are now"
                  catalogs={subject.library}
                  genres={genres}
                />
              )}
            </>
          )}
          {error && (
            <p role="alert" className="callout-danger type-data m-0">
              {error}
            </p>
          )}
        </div>
      </ModalBody>
      <ModalFooter tone="community">
        <button type="button" onClick={onClose} className="btn-ghost">
          Cancel
        </button>
        <button type="button" onClick={onConfirm} disabled={pending} className="btn-primary">
          {pending ? (update ? 'Publishing…' : 'Sharing…') : confirm}
        </button>
      </ModalFooter>
    </Modal>
  )
}

function CatalogGroup({ title, catalogs, genres }: { title: string; catalogs: Catalog[]; genres: GenreLookups }) {
  return (
    <div>
      <h3 className="type-label m-0">{title}</h3>
      <CatalogList items={catalogs.map((catalog) => groupItem(catalog, genres))} />
    </div>
  )
}

/** A catalog of a collection's group as a list row, marked From Community
 *  when it was added from there. */
function groupItem(catalog: Catalog, genres: GenreLookups): CatalogListItem {
  const item = catalogItem(catalog, genres)
  if (!catalog.subscription) return item
  return { ...item, sticker: FROM_COMMUNITY.label, tone: FROM_COMMUNITY.tone }
}
