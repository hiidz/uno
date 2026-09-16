import type { LucideIcon } from 'lucide-react'

/**
 * Every glyph in the app drawn with one line treatment — DESIGN.md's square
 * caps/joins, confirmed on Lucide's own `<svg>` root as inheritable and
 * override-safe (see the migration plan's Icon set decision). Callers pass a
 * Lucide icon component; nothing here is icon-specific.
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
      strokeWidth={1.5}
      strokeLinecap="square"
      strokeLinejoin="miter"
      className={className}
    />
  )
}

/**
 * The grip glyph: six 2px filled squares, DESIGN.md's own shape and the one
 * miss in Lucide's set (`grip`/`grip-vertical` are dots — see the migration
 * plan's Icon set decision). Hand-drawn rather than approximated with stroke
 * overrides, which can't turn a dot into a square.
 */
export function GripIcon({ size = 16, className }: { size?: number; className?: string }) {
  const squares = [2, 7, 12]
  return (
    <svg
      aria-hidden="true"
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="currentColor"
      className={className}
    >
      {squares.flatMap((y) => [
        <rect key={`l-${y}`} x={5} y={y} width={2} height={2} />,
        <rect key={`r-${y}`} x={9} y={y} width={2} height={2} />,
      ])}
    </svg>
  )
}
