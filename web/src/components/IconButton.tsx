import type { ComponentProps, MouseEvent } from 'react'
import type { LucideIcon } from 'lucide-react'
import { Icon } from './Icon'

const CIRCLE = {
  xs: { box: 'h-6 w-6', icon: 14 },
  sm: { box: 'h-8 w-8', icon: 16 },
  md: { box: 'h-9 w-9', icon: 17 },
} as const

/**
 * The one button that names itself by an icon (copy for duplicate, a bin for
 * delete), in two shapes: icon-only, or icon plus the first word of its
 * label. The label is its accessible name and tooltip either way.
 *
 * Every other button prop passes through to the `<button>`, `ref` included,
 * so a Radix trigger can render it through `asChild`.
 */
export function IconButton({
  label,
  icon,
  onClick,
  destructive,
  variant,
  size = 'md',
  className = '',
  ...rest
}: Omit<ComponentProps<'button'>, 'children'> & {
  label: string
  icon: LucideIcon
  destructive?: boolean
  /** `'icon'`: an icon-only circle printed on a sign — the editor header's own
   *  duplicate and delete, below `lg`, where the header is one compact line.
   *  `'plain'`: an icon-only circle on a pane or a row — a ⋯ menu, a row's
   *  ↑/↓ — grey until hovered, and dimmest with no hover while disabled.
   *  `'labeled'`: the icon plus the label's first word — the library rail's
   *  own row actions, above `lg`, where a selected row has room to spend. */
  variant: 'icon' | 'plain' | 'labeled'
  /** The circle's size: 24, 32 or 36px. The labeled shape has one size. */
  size?: keyof typeof CIRCLE
}) {
  const circle = CIRCLE[size]
  return (
    <button
      type="button"
      data-slot="icon-button"
      {...rest}
      onClick={(event: MouseEvent<HTMLButtonElement>) => {
        // Stops the click from also reaching whatever this button sits
        // inside. A no-op where nothing's listening (the editor header);
        // load-bearing where it is (a library row that selects itself).
        event.stopPropagation()
        onClick?.(event)
      }}
      aria-label={label}
      title={rest.title ?? label}
      className={`${
        variant === 'icon'
          ? `tap grid ${circle.box} shrink-0 place-items-center rounded-full transition-colors ${
              destructive ? 'hover:bg-sign-ink hover:text-danger' : 'hover:bg-sign-ink/15'
            }`
          : variant === 'plain'
          ? `tap grid ${circle.box} shrink-0 place-items-center rounded-full transition-colors text-dim enabled:hover:bg-line enabled:hover:text-ink enabled:active:bg-line-hi disabled:text-dimmer disabled:cursor-not-allowed`
          : `flex h-8 items-center gap-1.5 rounded-full px-3 text-meta font-semibold transition-[color,box-shadow] pointer-coarse:h-11 ${
              destructive
                ? 'text-dim shadow-[inset_0_0_0_1px_var(--uno-line-hi)] hover:text-danger hover:shadow-[inset_0_0_0_1.5px_var(--uno-danger)]'
                : 'text-dim shadow-[inset_0_0_0_1px_var(--uno-line-hi)] hover:text-ink hover:shadow-[inset_0_0_0_1px_var(--uno-dim)]'
            }`
      } ${className}`}
    >
      <Icon icon={icon} size={variant === 'labeled' ? 15 : circle.icon} />
      {variant === 'labeled' && label.split(' ')[0]}
    </button>
  )
}
