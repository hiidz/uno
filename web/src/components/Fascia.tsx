/**
 * The shop's fascia stripe: the four region colours side by side, in the
 * order the builder meets them — catalogs, collections, community, the TV.
 * Drawn only on the way in (the sign-in page, the profile picker and its
 * membership cards), never inside the builder, where each colour stands for
 * its own region.
 */
export function Fascia({ className = '' }: { className?: string }) {
  return (
    <div aria-hidden="true" className={`flex ${className}`}>
      <span className="bg-catalog flex-1" />
      <span className="bg-collection flex-1" />
      <span className="bg-community flex-1" />
      <span className="bg-tv-yellow flex-1" />
    </div>
  )
}
