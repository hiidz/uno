import type { ReactNode } from 'react'
import { ArrowUp } from 'lucide-react'
import { Icon } from './Icon'
import { Toast } from './Toast'
import { useToast } from './useToast'

/** DESIGN.md's Sign Title: a name the user typed, set on a pane's sign in
 *  Archivo at a narrower width and in the user's own case. The caller adds
 *  its margin, truncation and focus outline. */
export const SIGN_TITLE =
  'font-[family-name:var(--font-sign)] text-[18px] leading-tight font-extrabold [font-stretch:112%] lg:text-[25px]'

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

/** The next step a pane's subject waits on, named for what pressing it does. */
export interface SignStep {
  label: string
  /** Why the step can't be taken yet ("Save first."), or null. */
  waiting: string | null
  onClick: () => void
}

/**
 * A subject's next step with Community — Publish…, Unpublish… — as an
 * outlined pill in the editor's save bar. Greyed, not disabled, while the step
 * is `waiting`: pressing it says why in a one-line toast above the button.
 * Draws nothing without a step.
 */
export function SignStepButton({ step }: { step: SignStep | undefined }) {
  if (!step) return null
  return <StepButton step={step} />
}

function StepButton({ step: { label, waiting, onClick } }: { step: SignStep }) {
  const [toast, setToast] = useToast()

  function press() {
    if (waiting === null) {
      onClick()
      return
    }
    setToast({ text: waiting, tone: 'success' })
  }

  return (
    <span className="relative shrink-0">
      <button
        type="button"
        aria-disabled={waiting !== null}
        onClick={press}
        className="btn-secondary aria-disabled:cursor-not-allowed aria-disabled:text-dimmer"
      >
        {label}
      </button>
      <span className="absolute right-0 bottom-full z-30 mb-2 whitespace-nowrap">
        <Toast toast={toast} />
      </span>
    </span>
  )
}
