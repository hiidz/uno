import type { ReactNode } from 'react'
import { TriangleAlert } from 'lucide-react'
import { Icon } from '@/components/Icon'

/**
 * DESIGN.md's "Standing removal warning": a note stating what the next Save
 * does, with an Undo, since nothing has been written yet. `danger` is a saved
 * folder dropped from the tree, which the Save deletes server-side, cascading
 * its refs; `neutral` is a staged Move to library, which the catalog survives.
 */
export function StagedNote({
  tone,
  onUndo,
  children,
}: {
  tone: 'danger' | 'neutral'
  onUndo: () => void
  children: ReactNode
}) {
  return (
    <div className="setting flex-row gap-3">
      <span className="shrink-0 pt-0.5">
        <Icon icon={TriangleAlert} size={16} className={tone === 'danger' ? 'text-danger' : 'text-dim'} />
      </span>
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span className="text-[14px] leading-[20px]">{children}</span>
        <button type="button" onClick={onUndo} className="btn-ghost h-auto px-0 text-[13px]">
          Undo
        </button>
      </div>
    </div>
  )
}
