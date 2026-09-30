import { Check } from 'lucide-react'
import type { CommunityItem } from '@/api'
import { InfoTip } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { MoreMenu, MoreMenuItem } from '@/components/MoreMenu'
import type { CommunityAction } from './useCommunityMutations'

const PENDING_LABEL: Record<CommunityAction, string> = {
  take: 'Taking…',
  update: 'Updating…',
  duplicate: 'Duplicating…',
}

/** A row's or a publication page's actions. A row's `onUpdate` opens the
 *  publication's page, which shows the new version, and says so with
 *  `updateLabel` "Update…"; the page's applies it, as "Update". */
export interface RowActions {
  pending: CommunityAction | undefined
  onTake: () => void
  onUpdate: () => void
  updateLabel: string
  onDuplicate: () => void
}

/**
 * One main button, from the server's own flags — Take; a disabled "✓ Taken"
 * while this profile holds a copy that follows the publication; while an
 * update waits for that copy, the actions' `updateLabel` — and Duplicate, a
 * copy that is the profile's own and follows nothing, behind "⋯".
 */
export function ItemActions({ item, actions }: { item: CommunityItem; actions: RowActions }) {
  const { pending } = actions
  const taken = item.subscribed && !item.update_available
  return (
    <div className="flex shrink-0 items-center gap-1.5">
      <button
        type="button"
        onClick={item.update_available ? actions.onUpdate : actions.onTake}
        disabled={pending !== undefined || taken}
        className="btn-secondary btn-sm"
      >
        <MainLabel item={item} pending={pending} update={actions.updateLabel} />
      </button>
      <InfoTip
        label="Take"
        text="Take adds a copy to your library that follows its owner's updates until you save a change to it, which makes it yours. Duplicate (⋯) makes a copy that's yours from the start."
      />
      <MoreMenu label={item.title}>
        <MoreMenuItem disabled={pending !== undefined} onSelect={actions.onDuplicate}>
          Duplicate
        </MoreMenuItem>
      </MoreMenu>
    </div>
  )
}

/** The main button's words: what is in flight, else `update`, ✓ Taken or
 *  Take. */
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
  if (!item.subscribed) return 'Take'
  return (
    <>
      <Icon icon={Check} size={14} />
      Taken
    </>
  )
}

/**
 * One Community row: its name with its kind, and an Update sticker while an
 * update waits for this profile's copy; what it is in plain words; how many
 * have taken it and when it last changed. The name, summary and meta are one
 * button that opens the publication's page. No owner anywhere: Community
 * never names who shared a row.
 */
export function CommunityRow({
  item,
  summary,
  meta,
  buttonID,
  onOpen,
  actions,
}: {
  item: CommunityItem
  summary: string
  meta: string
  /** The open button's id, which focus returns to when its page closes. */
  buttonID: string
  onOpen: () => void
  actions: RowActions
}) {
  return (
    <div className="border-line flex items-center gap-3 border-b py-3.5">
      <button
        id={buttonID}
        type="button"
        onClick={onOpen}
        className="flex min-w-0 flex-1 flex-col gap-1 rounded-[8px] text-left"
      >
        <span className="flex min-w-0 items-center gap-2">
          <span className="truncate text-[16px] font-bold">{item.title}</span>
          <span className="stk stk-kind shrink-0">{item.kind === 'catalog' ? 'Catalog' : 'Collection'}</span>
          {item.update_available && <span className="stk stk-update shrink-0">Update</span>}
        </span>
        <span className="text-dim truncate text-[13.5px]">{summary}</span>
        <span className="type-data text-dimmer text-[12.5px]">{meta}</span>
      </button>
      <ItemActions item={item} actions={actions} />
    </div>
  )
}
