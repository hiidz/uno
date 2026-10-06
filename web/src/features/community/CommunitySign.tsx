import { useEffect, useRef } from 'react'
import { ArrowLeft } from 'lucide-react'
import type { CommunityItem } from '@/api'
import { Icon } from '@/components/Icon'
import { SIGN_TITLE } from '@/components/PaneSign'
import { PageStickers } from './PublicationPage'

/**
 * Community's sign. On the list it names the region. With a publication's page
 * open it becomes that page's sign: a round way back outlined in sign ink, then
 * the publication's name set as an editor's header sign sets its name
 * (DESIGN.md's Sign Title, in the user's own case), then its kind from `sm`
 * up; below `sm` it heads the page's body.
 */
export function CommunitySign({ open, onBack }: { open: CommunityItem | null; onBack: () => void }) {
  return (
    <div className="sign min-h-[64px] gap-3 px-4 py-3 lg:min-h-[80px] lg:px-6">
      {open ? (
        <PageSign key={open.id} item={open} onBack={onBack} />
      ) : (
        <h1 className="type-sign m-0 text-[18px] leading-tight lg:text-[25px]">Community</h1>
      )}
    </div>
  )
}

/** A publication page's sign. Focus lands on the title when the page opens, and
 *  Escape leaves it, as the arrow does (DESIGN.md's One Way Back rule). */
function PageSign({ item, onBack }: { item: CommunityItem; onBack: () => void }) {
  const headingRef = useRef<HTMLHeadingElement>(null)

  useEffect(() => headingRef.current?.focus(), [])

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== 'Escape' || event.defaultPrevented || document.querySelector('[role="dialog"]')) return
      onBack()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [onBack])

  return (
    <>
      <button
        type="button"
        onClick={onBack}
        aria-label="Back to Community"
        className="sign-btn-outline tap w-9 shrink-0 justify-center px-0"
      >
        <Icon icon={ArrowLeft} size={18} />
      </button>
      <h1
        ref={headingRef}
        tabIndex={-1}
        className={`m-0 min-w-0 outline-none ${SIGN_TITLE}`}
      >
        {item.title}
      </h1>
      <div className="hidden shrink-0 items-center gap-1.5 sm:flex">
        <PageStickers item={item} />
      </div>
    </>
  )
}
