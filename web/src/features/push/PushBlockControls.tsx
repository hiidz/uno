import { TriangleAlert } from 'lucide-react'
import { Icon } from '@/components/Icon'
import type { PushBlock } from './pushBlock'
import { PushButton } from './PushControls'
import type { Push } from './usePush'

interface BlockablePushButtonProps {
  push: Push
  block: PushBlock | null
  className?: string
}

/** The Push button, also disabled while something blocks the push
 *  (`pushBlock`), which `PushBlockNote` says beneath the header. */
export function BlockablePushButton({ push, block, className }: BlockablePushButtonProps) {
  const ready = push.ready && block === null
  return (
    <PushButton
      push={push.push}
      ready={ready}
      pushing={push.pushing}
      outcome={push.outcome}
      dismiss={push.dismiss}
      className={className}
    />
  )
}

interface PushBlockNoteProps {
  block: PushBlock | null
}

/**
 * What blocks the push, in a strip under the header for as long as it does: a
 * disabled Push needs its reason beside it, and the reason is what to do.
 */
export function PushBlockNote({ block }: PushBlockNoteProps) {
  if (!block) return null
  const words = blockWords(block)

  return (
    <div role="status" className="bg-raised border-line flex shrink-0 items-start gap-3 border-b px-4 py-3 lg:px-5">
      <span aria-hidden="true" className="bg-pending text-sign-ink grid size-6 shrink-0 place-items-center rounded-full">
        <Icon icon={TriangleAlert} size={14} />
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <p className="text-ink m-0 text-[14px] font-bold">{words.headline}</p>
        <p className="text-dim m-0 text-[13.5px]">{words.detail}</p>
      </div>
    </div>
  )
}

interface BlockWords {
  headline: string
  detail: string
}

function blockWords(block: PushBlock): BlockWords {
  if (block.kind === 'shares-addons') return SHARES_ADDONS_WORDS
  return emptyCollectionWords(block.title)
}

const SHARES_ADDONS_WORDS: BlockWords = {
  headline: "Push is off: this profile uses profile 1's addons in Nuvio.",
  detail: 'Nothing pushed from here would show in Nuvio. Turn sharing off for this profile in Nuvio, then pick it again.',
}

function emptyCollectionWords(title: string): BlockWords {
  return {
    headline: 'Push is off: “' + title + '” has no folders.',
    detail: "Nuvio can't show a collection without folders. Add a folder to it, or take it off Home.",
  }
}
