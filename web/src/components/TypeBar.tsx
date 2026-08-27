import { cn } from '@/lib/utils'

export type BarKind = 'movie' | 'series' | 'collection'

const HUE: Record<BarKind, string> = {
  movie: 'text-movie',
  series: 'text-series',
  collection: 'text-collection',
}

/**
 * The design's signature element, and the only place type and ownership are
 * encoded visually: a full-height bar at the left edge of a row, hue by kind,
 * solid when the row is yours and hatched when it belongs to the community.
 * Read down a list, the column of bars is a test pattern.
 *
 * Both renderings draw from `currentColor`, so the hue class is the single
 * source of the colour — `bg-current` fills it, `.hatch` stripes it.
 *
 * Deliberately `aria-hidden`: it carries no information a screen reader can
 * use. Every row that uses it must also state its type and ownership in text
 * (the owner column does this today), so the bar stays a fast visual channel
 * rather than the only channel.
 */
export function TypeBar({
  kind,
  owned,
  className,
}: {
  kind: BarKind
  owned: boolean
  className?: string
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        'w-[3px] shrink-0 self-stretch rounded-[1px]',
        HUE[kind],
        owned ? 'bg-current' : 'hatch',
        className,
      )}
    />
  )
}
