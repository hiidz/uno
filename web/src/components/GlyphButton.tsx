import type { LucideIcon } from 'lucide-react'
import { Icon } from './Icon'

/**
 * A small button naming itself by an icon (copy for duplicate, a bin for
 * delete), in the two shapes this app draws one at: icon-only, or icon plus
 * the first word of its label.
 */
export function GlyphButton({
  label,
  icon,
  onClick,
  destructive,
  variant,
}: {
  label: string
  icon: LucideIcon
  onClick: () => void
  destructive?: boolean
  /** `'icon'`: an icon-only circle printed on a sign — the editor header's own
   *  duplicate and delete, below `lg`, where the header is one compact line.
   *  `'labeled'`: the icon plus the label's first word — the library rail's
   *  own row actions, above `lg`, where a selected row has room to spend. */
  variant: 'icon' | 'labeled'
}) {
  return (
    <button
      type="button"
      onClick={(event) => {
        // Stops the click from also reaching whatever this button sits
        // inside. A no-op where nothing's listening (the editor header);
        // load-bearing where it is (a library row that selects itself).
        event.stopPropagation()
        onClick()
      }}
      aria-label={label}
      title={label}
      className={
        variant === 'icon'
          ? `tap grid h-9 w-9 shrink-0 place-items-center rounded-full transition-colors ${
              destructive ? 'hover:bg-sign-ink hover:text-danger' : 'hover:bg-sign-ink/15'
            }`
          : `flex h-8 items-center gap-1.5 rounded-full px-3 text-[13px] font-semibold transition-[color,box-shadow] pointer-coarse:h-11 ${
              destructive
                ? 'text-dim shadow-[inset_0_0_0_1px_var(--uno-line-hi)] hover:text-danger hover:shadow-[inset_0_0_0_1.5px_var(--uno-danger)]'
                : 'text-dim shadow-[inset_0_0_0_1px_var(--uno-line-hi)] hover:text-ink hover:shadow-[inset_0_0_0_1px_var(--uno-dim)]'
            }`
      }
    >
      <Icon icon={icon} size={variant === 'icon' ? 17 : 15} />
      {variant === 'labeled' && label.split(' ')[0]}
    </button>
  )
}
