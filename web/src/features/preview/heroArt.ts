import type { TileShape } from '@/api'

/**
 * How Nuvio TV's Modern Home draws a folder's hero (NuvioTV `ModernHomeModels.kt`,
 * `ModernHomeHero.kt`): the backdrop is the folder's hero backdrop, else its
 * cover, else the collection's background image; over it, the title logo, or
 * the title as text — the cover emoji, two spaces, then the title — and
 * nothing at all when the folder hides its title.
 */
export interface HeroArt {
  /** The backdrop address, or `''` for none. */
  backdrop: string
  /** The title logo's address, or `''` for none. */
  logo: string
  /** What shows where the logo goes when there is none or it fails to load:
   *  `''` when the folder hides its title. */
  text: string
}

export function heroArt(folder: {
  title: string
  hideTitle: boolean
  coverEmoji: string
  coverImageURL: string
  heroBackdropURL: string
  titleLogoURL: string
  /** The collection's background image. */
  backdropImageURL: string
}): HeroArt {
  const backdrop =
    [folder.heroBackdropURL, folder.coverImageURL, folder.backdropImageURL].map((url) => url.trim()).find(Boolean) ??
    ''
  const title = folder.title.trim()
  const emoji = folder.coverEmoji.trim()
  const text = folder.hideTitle ? '' : emoji ? `${emoji}  ${title}` : title
  return { backdrop, logo: folder.titleLogoURL.trim(), text }
}

/** The site `address` names, lower-cased, or `''` when it names none. */
export function siteOf(address: string): string {
  try {
    return new URL(address).hostname.toLowerCase()
  } catch {
    return ''
  }
}

/** The `.pv-folder` class a tile shape draws with. */
export function shapeClass(shape: TileShape): 'poster' | 'landscape' | 'square' {
  return shape === 'LANDSCAPE' ? 'landscape' : shape === 'SQUARE' ? 'square' : 'poster'
}
