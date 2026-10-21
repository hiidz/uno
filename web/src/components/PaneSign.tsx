import type { ReactNode } from 'react'
import { ArrowUp } from 'lucide-react'
import { Button } from './Button'
import { Icon } from './Icon'

/** DESIGN.md's Sign Title: a name the user typed, set on a pane's sign in
 *  Archivo at a narrower width and in the user's own case, on two lines at
 *  most and broken anywhere a long word needs. The caller adds its margin
 *  and focus outline. */
export const SIGN_TITLE =
  'font-[family-name:var(--font-sign)] text-sign-title font-extrabold [font-stretch:112%] line-clamp-2 [overflow-wrap:anywhere]'

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
    <Button kind="sign-outline" onClick={onClick} title={title} className={`tap ${className}`}>
      <Icon icon={ArrowUp} size={15} />
      Library
    </Button>
  )
}
