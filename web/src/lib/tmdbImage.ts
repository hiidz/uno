/** The size TMDB poster URLs reach the SPA at (`internal/provider`), sized
 *  for Nuvio's own tiles. */
const W500 = 'https://image.tmdb.org/t/p/w500/'

/** A TMDB person's headshot from their raw `profile_path`, at a width that
 *  stays sharp as a small round avatar; `''` when they have none. */
export function headshot(profilePath: string): string {
  return profilePath ? `https://image.tmdb.org/t/p/w185${profilePath}` : ''
}

/** The TMDB poster widths a thumbnail picks from. */
const WIDTHS = [92, 154, 185, 342]

/** The `<img>` attributes for a thumbnail of `url`, drawn `sizes` wide. A
 *  TMDB `w500` poster becomes a `srcset` of smaller TMDB sizes, so the
 *  browser fetches the one the tile needs; any other URL is used as it is. */
export function thumbnail(url: string, sizes: string): { src: string; srcSet?: string; sizes?: string } {
  if (!url.startsWith(W500)) return { src: url }
  const file = url.slice(W500.length)
  const at = (width: number) => `https://image.tmdb.org/t/p/w${width}/${file}`
  return { src: at(185), srcSet: WIDTHS.map((width) => `${at(width)} ${width}w`).join(', '), sizes }
}
