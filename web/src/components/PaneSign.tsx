import type { ReactNode } from 'react'
import { ArrowUp } from 'lucide-react'
import { Icon } from './Icon'

/**
 * The sign over the builder's pane — the Home pane's, and each editor's.
 * Below `lg` it pins under the app header (`--app-h`, published by
 * `stacked.ts`): it names the region the page has just scrolled to and carries
 * the way back up out of it. From `lg` the pane's own column holds it in
 * place, so it is static there.
 */
export function PaneSign({
  as: Tag = 'div',
  className = '',
  children,
}: {
  as?: 'div' | 'header'
  className?: string
  children: ReactNode
}) {
  return (
    <Tag
      className={`sign sticky top-[var(--app-h)] z-20 px-4 py-2.5 lg:static lg:min-h-[80px] lg:px-6 lg:py-4 ${className}`}
    >
      {children}
    </Tag>
  )
}

/** The way back up to the Library rail, printed on a pane's sign below `lg`. */
export function SignLibraryButton({
  onClick,
  title,
  className = '',
}: {
  onClick: () => void
  title: string
  className?: string
}) {
  return (
    <button type="button" onClick={onClick} title={title} className={`tap sign-btn-outline ${className}`}>
      <Icon icon={ArrowUp} size={15} />
      Library
    </button>
  )
}
