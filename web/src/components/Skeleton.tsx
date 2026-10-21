/** A blank bar standing in for a line, a sticker or a picture while a list
 *  loads. Decorative: `ListState` says "Loading…" to screen readers. */
export function SkeletonBar({ className }: { className: string }) {
  return <span className={`bg-raised-hi block shrink-0 animate-pulse rounded-md motion-reduce:animate-none ${className}`} />
}

/** The keys for `count` copies of a list's skeleton row. */
export function skeletonKeys(count: number): number[] {
  return Array.from({ length: count }, (_, i) => i)
}
