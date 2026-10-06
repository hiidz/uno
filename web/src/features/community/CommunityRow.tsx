import { Check, ChevronRight } from 'lucide-react'
import type { CommunityItem } from '@/api'
import { Icon } from '@/components/Icon'
import { MoreMenu, MoreMenuItem } from '@/components/MoreMenu'
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
 * A row's actions, from the server's own flags: one main button — Add; a
 * disabled "✓ Added" while this profile holds a copy that follows the
 * publication; while an update waits for that copy, the actions' `updateLabel`
 * — and Duplicate, a copy that is the profile's own and follows nothing, behind
 * "⋯", with what it makes said beside it (`duplicateReason`).
 */
function ItemActions({ item, actions }: { item: CommunityItem; actions: RowActions }) {
  const { pending } = actions
  const added = item.subscribed && !item.update_available
  return (
    <div className="flex shrink-0 items-center gap-1.5">
      <button
        type="button"
        onClick={item.update_available ? actions.onUpdate : actions.onSubscribe}
        disabled={pending !== undefined || added}
        className={mainButtonClass(item)}
      >
        <MainLabel item={item} pending={pending} update={actions.updateLabel} />
      </button>
      <MoreMenu label={item.title}>
        <MoreMenuItem disabled={pending !== undefined} onSelect={actions.onDuplicate} reason={duplicateReason(item)}>
          Duplicate
        </MoreMenuItem>
      </MoreMenu>
    </div>
  )
}

/** The row's main button: outlined, and outlined in the Community accent while
 *  an update waits, which is how the row says one does. */
function mainButtonClass(item: CommunityItem): string {
  if (item.update_available) return 'btn-secondary btn-sm btn-accent-outline'
  return 'btn-secondary btn-sm'
}

/** What Duplicate makes, beside it in the row's "⋯": the latest version while
 *  an update waits for this profile's older added row, otherwise a copy that
 *  is yours to edit. */
function duplicateReason(item: CommunityItem): string {
  return item.update_available ? 'the latest version' : 'yours to edit'
}

/**
 * A publication page's actions: the one primary, in the Community accent —
 * Add, Update while one waits, or a disabled ✓ Added — and beside it
 * Duplicate. The page says in its own words what each makes, so there is no
 * menu and no tip.
 */
export function PageActions({ item, actions }: { item: CommunityItem; actions: RowActions }) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <PagePrimary item={item} actions={actions} />
      <button type="button" onClick={actions.onDuplicate} disabled={actions.pending !== undefined} className="btn-secondary">
        {actions.pending === 'duplicate' ? PENDING_LABEL.duplicate : 'Duplicate'}
      </button>
    </div>
  )
}

/** The page's one primary: Add, Update, or a disabled outlined ✓ Added. */
function PagePrimary({ item, actions }: { item: CommunityItem; actions: RowActions }) {
  const added = item.subscribed && !item.update_available
  return (
    <button
      type="button"
      onClick={mainClick(item, actions)}
      disabled={actions.pending !== undefined || added}
      className={added ? 'btn-secondary' : 'btn-primary'}
    >
      <MainLabel item={item} pending={settingAside(actions.pending, 'duplicate')} update={actions.updateLabel} />
    </button>
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
  if (pending) return PENDING_LABEL[pending]
  if (item.update_available) return update
  if (!item.subscribed) return 'Add'
  return (
    <>
      <Icon icon={Check} size={14} />
      Added
    </>
  )
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
    <div className="community-row hover:bg-raised active:bg-raised-hi relative -mx-3 flex items-start gap-3 rounded-[12px] px-3 py-3 transition-colors sm:items-center sm:gap-4">
      <RowPosters item={item} />
      <div className="flex min-w-0 flex-1 flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex min-w-0 flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-2">
            <button
              id={buttonID}
              type="button"
              onClick={onOpen}
              className="flex min-w-0 cursor-pointer items-start gap-2 text-left outline-none after:absolute after:inset-0 after:rounded-[12px] focus-visible:after:outline-2 focus-visible:after:-outline-offset-2 focus-visible:after:outline-ink"
            >
              <span className="line-clamp-2 min-w-0 flex-1 text-[16px] font-bold [overflow-wrap:anywhere] sm:truncate">{item.title}</span>
              <Icon icon={ChevronRight} size={16} className="text-dimmer mt-[3px] hidden shrink-0 pointer-coarse:block" />
            </button>
            <RowStickers item={item} showKind={showKind} />
          </div>
          <span className="text-dim line-clamp-2 text-[13.5px] sm:line-clamp-1">{summary}</span>
          <RowFolders item={item} onOpen={onOpen} />
          <span className="type-data text-dimmer text-[12.5px]">
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
