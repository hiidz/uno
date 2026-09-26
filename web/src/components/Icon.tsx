import type { LucideIcon } from 'lucide-react'

/**
 * Every glyph in the app drawn with one line treatment: a 2px stroke with
 * round caps and joins, set on Lucide's own `<svg>` root, where it is
 * inheritable and override-safe. Callers pass a Lucide icon component;
 * nothing here is icon-specific.
 */
export function Icon({
  icon: LucideGlyph,
  size = 16,
  className,
}: {
  icon: LucideIcon
  size?: number
  className?: string
}) {
  return (
    <LucideGlyph
      aria-hidden="true"
      size={size}
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
    />
  )
}

/**
 * The drag grip: two columns of three dots, drawn filled so they hold their
 * weight at 16px where a stroked circle thins out.
 */
export function GripIcon({ size = 16, className }: { size?: number; className?: string }) {
  const rows = [3, 8, 13]
  return (
    <svg
      aria-hidden="true"
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="currentColor"
      className={className}
    >
      {rows.flatMap((y) => [
        <circle key={`l-${y}`} cx={5.5} cy={y} r={1.4} />,
        <circle key={`r-${y}`} cx={10.5} cy={y} r={1.4} />,
      ])}
    </svg>
  )
}
