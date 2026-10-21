import { Check, ChevronRight } from 'lucide-react'
import type { CommunityItem } from '@/api'
import { Button } from '@/components/Button'
import { Icon } from '@/components/Icon'
import { SkeletonBar } from '@/components/Skeleton'
import { SharingStickers } from '@/features/sharing/SharingStickers'
import { itemKind } from './communityQuery'
import { MetaParts } from './MetaParts'
import { RowFolders, RowPosters } from './RowPreview'
import type { CommunityAction } from './useCommunityMutations'

const PENDING_LABEL: Record<CommunityAction, string> = {
  subscribe: 'Adding…',
  update: 'Updating…',
  duplicate: 'Duplicating…',
}

/** A row's or a publication page's actions. A row's `onUpdate` opens the
 *  publication's page, which shows the new version, and says so with
 *  `updateLabel` "Update…"; the page's applies it, as "Update". */
export interface RowActions {
  pending: CommunityAction | undefined
  onSubscribe: () => void
  onUpdate: () => void
  updateLabel: string
  onDuplicate: () => void
}

/**
 * A row's one action, from the server's own flags: Add; a disabled "✓ Added"
 * while this profile holds a copy that follows the publication; while an
 * update waits for that copy, the actions' `updateLabel`, outlined in the
 * Community accent, which is how the row says one waits. Duplicate is the
 * publication page's.
 */
function ItemActions({ item, actions }: { item: CommunityItem; actions: RowActions }) {
  const { pending } = actions
  const added = item.subscribed && !item.update_available
  return (
    <Button
      kind={item.update_available ? 'accent-outline' : 'secondary'}
      size="sm"
      onClick={mainClick(item, actions)}
      disabled={pending !== undefined || added}
      // Every row's button reads Add, so its name also says which row.
      aria-label={`${mainWords(item, pending, actions.updateLabel)} ${item.title}`}
      className="shrink-0"
    >
      <MainLabel item={item} pending={pending} update={actions.updateLabel} />
    </Button>
  )
}

/** What the profile's own publication page does: publish the edits made
 *  since (only while `changed`), open the row in Configure, unpublish. */
export interface OwnPageActions {
  changed: boolean
  onPublish: () => void
  onOpen: () => void
  onUnpublish: () => void
}

/**
 * A publication page's actions: the one primary, in the Community accent —
 * Add, Update while one waits, or a disabled ✓ Added — and beside it
 * Duplicate. The page says in its own words what each makes, so there is no
 * menu and no tip. The profile's own publication, which it can neither add
 * nor duplicate, takes `own`'s instead; a line saying it published this
 * stands in while the library hasn't listed the row.
 */
export function PageActions({
  item,
  actions,
  own,
}: {
  item: CommunityItem
  actions: RowActions
  own?: OwnPageActions | null
}) {
  if (item.own) return own ? <OwnActions own={own} /> : <p className="text-dim text-body-sm m-0">You published this.</p>
  return (
    <div className="flex flex-wrap items-center gap-2">
      <PagePrimary item={item} actions={actions} />
      <Button kind="secondary" onClick={actions.onDuplicate} disabled={actions.pending !== undefined}>
        {actions.pending === 'duplicate' ? PENDING_LABEL.duplicate : 'Duplicate'}
      </Button>
    </div>
  )
}

/** Publish changes… as the one primary while edits wait, Open in Configure
 *  beside it, and Unpublish… in red text at the end. */
function OwnActions({ own }: { own: OwnPageActions }) {
  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-3">
      {own.changed && (
        <Button kind="primary" onClick={own.onPublish}>
          Publish changes…
        </Button>
      )}
      <Button kind="secondary" onClick={own.onOpen}>
        Open in Configure
      </Button>
      <Button kind="danger-text" onClick={own.onUnpublish} className="ml-2">
        Unpublish…
      </Button>
    </div>
  )
}

/** The page's one primary: Add, Update, or a disabled outlined ✓ Added. */
function PagePrimary({ item, actions }: { item: CommunityItem; actions: RowActions }) {
  const added = item.subscribed && !item.update_available
  return (
    <Button
      kind={added ? 'secondary' : 'primary'}
      onClick={mainClick(item, actions)}
      disabled={actions.pending !== undefined || added}
    >
      <MainLabel item={item} pending={settingAside(actions.pending, 'duplicate')} update={actions.updateLabel} />
    </Button>
  )
}

/** What the main button does: apply the update that waits, or Add. */
function mainClick(item: CommunityItem, actions: RowActions): () => void {
  return item.update_available ? actions.onUpdate : actions.onSubscribe
}

/** `pending`, unless it is `other`, which the main button does not show. */
function settingAside(pending: CommunityAction | undefined, other: CommunityAction): CommunityAction | undefined {
  return pending === other ? undefined : pending
}

/** The main button's words: what is in flight, else `update`, ✓ Added or
 *  Add. */
function MainLabel({
  item,
  pending,
  update,
}: {
  item: CommunityItem
  pending: CommunityAction | undefined
  update: string
}) {
  if (!item.subscribed || item.update_available || pending) return mainWords(item, pending, update)
  return (
    <>
      <Icon icon={Check} size={14} />
      Added
    </>
  )
}

/** The main button's words as text: what is in flight, else `update`, Add or
 *  Added. */
function mainWords(item: CommunityItem, pending: CommunityAction | undefined, update: string): string {
  if (pending) return PENDING_LABEL[pending]
  if (item.update_available) return update
  return item.subscribed ? 'Added' : 'Add'
}

interface CommunityRowProps {
  item: CommunityItem
  summary: string
  meta: string[]
  showKind: boolean
  /** The open button's id, which focus returns to when its page closes. */
  buttonID: string
  onOpen: () => void
  actions: RowActions
}

/**
 * One Community row: its name with its kind (Movies or Series; a collection's
 * list says it already, so `showKind` is off there); what it is in plain
 * words; how many have added it and when it last changed. A catalog
 * fans its first posters ahead of all that, and a collection lines up its
 * folder tiles under its summary (`RowPreview`). The row navigates, so it
 * takes a pointer and a fill on hover and the name button covers it; its
 * actions sit above that and act. No publisher anywhere: Community never
 * names who published a row.
 *
 * Below `sm` the row stacks beside its posters — name, stickers, summary,
 * meta, actions — and a touch screen, which has no hover, ends the name with
 * a chevron.
 */
export function CommunityRow({ item, summary, meta, showKind, buttonID, onOpen, actions }: CommunityRowProps) {
  return (
    <div className="community-row hover:bg-raised active:bg-raised-hi relative -mx-3 flex items-start gap-3 rounded-[12px] px-3 py-3 transition-colors max-[18rem]:flex-col sm:items-center sm:gap-4">
      <RowPosters item={item} />
      <div className="flex min-w-0 flex-1 flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex min-w-0 flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-2">
            {/* drift-ok: the row's name, its button stretched over the whole row */}
            <button
              id={buttonID}
              type="button"
              onClick={onOpen}
              className="flex min-w-0 cursor-pointer items-start gap-2 text-left outline-none after:absolute after:inset-0 after:rounded-[12px] focus-visible:after:outline-2 focus-visible:after:-outline-offset-2 focus-visible:after:outline-ink"
            >
              <span className="text-title min-w-0 flex-1 [overflow-wrap:anywhere]">{item.title}</span>
              <Icon icon={ChevronRight} size={16} className="text-dimmer mt-[3px] hidden shrink-0 pointer-coarse:block" />
            </button>
            <RowStickers item={item} showKind={showKind} />
          </div>
          <span title={summary} className="text-dim text-meta line-clamp-2 sm:line-clamp-1">
            {summary}
          </span>
          <RowFolders item={item} onOpen={onOpen} />
          <span className="type-data text-dimmer text-meta">
            <MetaParts parts={meta} />
          </span>
        </div>
        <div className="relative z-10 self-start sm:self-auto">
          <ItemActions item={item} actions={actions} />
        </div>
      </div>
    </div>
  )
}

/** A row's sticker: its kind, unless the list says it already; nothing
 *  otherwise. Its outlined pink Update… says an update waits. */
function RowStickers({ item, showKind }: { item: CommunityItem; showKind: boolean }) {
  if (!showKind) return null
  return (
    <span className="flex flex-wrap items-center gap-1.5">
      <SharingStickers stickers={[itemKind(item)]} />
    </span>
  )
}

/** A row's stand-in while Community loads: `CommunityRow`'s padding and
 *  height at each width, its poster stack, and its name, summary and meta
 *  lines. */
export function CommunityRowSkeleton() {
  return (
    <div className="-mx-3 flex h-[173px] items-start gap-3 px-3 py-3 sm:h-[95px] sm:items-center sm:gap-4">
      <SkeletonBar className="h-[60px] w-[76px] sm:h-[69px] sm:w-[106px]" />
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <SkeletonBar className="h-4 w-2/5" />
        <SkeletonBar className="h-3 w-3/4" />
        <SkeletonBar className="h-3 w-1/4" />
      </div>
    </div>
  )
}
