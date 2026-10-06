import { useRef } from 'react'

/** Rows the field shows when it holds less, and the share of the screen it grows to. */
const MIN_LINES = 8
const MAX_HEIGHT = '60vh'

const TEXT = 'font-mono text-[13px] leading-snug pointer-coarse:text-[16px]'

/**
 * A multi-line field for JSON with a line number beside each line. It grows
 * with its text up to `MAX_HEIGHT`, then scrolls. Lines never wrap, so a
 * number always sits on its line, and the gutter scrolls with the text. It
 * takes focus when it mounts: wherever it appears, the next keystroke is
 * meant for it.
 */
export function JsonField({
  value,
  readOnly,
  invalid,
  ariaLabel,
  placeholder,
  onChange,
}: {
  value: string
  readOnly: boolean
  invalid: boolean
  ariaLabel: string
  placeholder: string
  onChange: (value: string) => void
}) {
  const gutter = useRef<HTMLDivElement>(null)
  const lines = Math.max(value.split('\n').length, MIN_LINES)
  const numbers = Array.from({ length: lines }, (_, i) => i + 1).join('\n')

  return (
    <div
      className={`bg-raised flex w-full overflow-hidden rounded-[10px] border focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-[var(--uno-ink)] ${
        invalid ? 'border-[var(--uno-danger)]' : 'border-line-hi'
      } ${TEXT}`}
      style={{ height: `min(${MAX_HEIGHT}, calc(${lines}lh + 20px))` }}
    >
      <div
        ref={gutter}
        aria-hidden
        className="text-dimmer border-line shrink-0 overflow-hidden border-r px-2.5 py-2.5 text-right whitespace-pre select-none"
      >
        {numbers}
      </div>
      <textarea
        autoFocus
        value={value}
        readOnly={readOnly}
        spellCheck={false}
        wrap="off"
        placeholder={placeholder}
        aria-label={ariaLabel}
        aria-invalid={invalid || undefined}
        onChange={(event) => onChange(event.target.value)}
        onScroll={(event) => {
          if (gutter.current) gutter.current.scrollTop = event.currentTarget.scrollTop
        }}
        className={`text-ink min-w-0 flex-1 resize-none overflow-auto bg-transparent px-3 py-2.5 whitespace-pre outline-none ${TEXT}`}
      />
    </div>
  )
}
