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

/** Where a `SignStepButton` sits: on the sign from `sm` up, or in the sticker
 *  row heading the body below `sm`, where the sign leaves stickers out. */
type StepPlace = 'sign' | 'body'

const STEP_PLACE: Record<StepPlace, { wrap: string; button: string; toast: string }> = {
  sign: {
    wrap: 'hidden sm:block',
    button:
      'sign-btn-outline h-[26px] px-[11px] text-[12.5px] pointer-coarse:h-9 pointer-coarse:px-3.5 aria-disabled:cursor-not-allowed aria-disabled:opacity-55 aria-disabled:hover:bg-transparent',
    toast: 'right-0',
  },
  body: {
    wrap: 'sm:hidden',
    button: 'btn-secondary btn-sm aria-disabled:cursor-not-allowed aria-disabled:text-dimmer',
    toast: 'left-0',
  },
}

/** The next step a pane's subject waits on, named for what pressing it does. */
export interface SignStep {
  label: string
  /** Why the step can't be taken yet ("Save first."), or null. */
  waiting: string | null
  onClick: () => void
}

/**
 * A pane's next step as a button on its sign — Publish…, Unpublish… —
 * outlined in sign ink (DESIGN.md's "Controls on a sign"). Greyed, not
 * disabled, while the step is `waiting`: pressing it says why in a one-line
 * toast under the button. Draws nothing without a step.
 */
export function SignStepButton({ step, place }: { step: SignStep | undefined; place: StepPlace }) {
  if (!step) return null
  return <StepButton step={step} place={place} />
}

function StepButton({ step: { label, waiting, onClick }, place }: { step: SignStep; place: StepPlace }) {
  const [toast, setToast] = useToast()

  function press() {
    if (waiting === null) {
      onClick()
      return
    }
    setToast({ text: waiting, tone: 'success' })
  }

  return (
    <span className={`relative shrink-0 ${STEP_PLACE[place].wrap}`}>
      <button type="button" aria-disabled={waiting !== null} onClick={press} className={STEP_PLACE[place].button}>
        {label}
      </button>
      <span className={`absolute top-full z-30 mt-2 whitespace-nowrap ${STEP_PLACE[place].toast}`}>
        <Toast toast={toast} />
      </span>
    </span>
  )
}
