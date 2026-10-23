import { useState } from 'react'
import type { TileShape } from '@/api'
import { prefersReducedMotion } from '@/lib/motion'
import { shapeClass, siteOf, type HeroArt } from './heroArt'

/** Whether the picture at `src` failed to load, reset for each new `src`: a
 *  fixed address doesn't keep the last one's failure. */
function useFailed(src: string): [boolean, () => void] {
  const [failedSrc, setFailedSrc] = useState<string | null>(null)
  return [failedSrc === src, () => setFailedSrc(src)]
}

/**
 * A folder picture as a tile: the cover at the folder's tile shape, or, with
 * no `shape`, at its own proportions (a title logo). The raised card alone
 * shows while it loads; a picture that fails says so in its place. Loaded
 * only when drawn, and lazily: callers mount it only where it shows.
 */
export function PictureTile({ src, shape }: { src: string; shape?: TileShape }) {
  const [failed, fail] = useFailed(src)
  return (
    <span className={`art-pic ${shape ? shapeClass(shape) : 'free'}${failed ? ' is-failed' : ''}`}>
      {failed ? (
        <span className="art-pic-note text-caption">Couldn’t load this picture</span>
      ) : (
        <img src={src} alt="" loading="lazy" decoding="async" onError={fail} />
      )}
    </span>
  )
}

/**
 * A focus GIF at the folder's tile shape, which Nuvio TV draws in place of
 * the cover while the tile is focused. A GIF can't be paused once drawn, so
 * on a device asking for less motion it isn't loaded: the tile says one is
 * set, and from which site.
 */
export function FocusGIFTile({ src, shape }: { src: string; shape: TileShape }) {
  if (!prefersReducedMotion()) return <PictureTile src={src} shape={shape} />
  const site = siteOf(src)
  return (
    <span className={`art-pic ${shapeClass(shape)} is-still`}>
      <span className="art-pic-note text-caption">
        <span className="text-ink font-semibold">Set</span>
        {site && <span className="break-all">{site}</span>}
      </span>
    </span>
  )
}

/**
 * A folder's hero roughly as Nuvio TV's Modern Home draws it: the backdrop,
 * shaded from the left and from the bottom, with the title logo at the left,
 * or the title's text where there is no logo or it fails to load.
 */
export function HeroPreview({ art }: { art: HeroArt }) {
  const [backdropFailed, failBackdrop] = useFailed(art.backdrop)
  const [logoFailed, failLogo] = useFailed(art.logo)
  const showLogo = art.logo !== '' && !logoFailed
  return (
    <div className="art-hero" role="img" aria-label={`Hero preview${art.text ? `: ${art.text}` : ''}`}>
      {art.backdrop && !backdropFailed && (
        <img className="art-hero-backdrop" src={art.backdrop} alt="" loading="lazy" decoding="async" onError={failBackdrop} />
      )}
      <span className="art-hero-shade" aria-hidden="true" />
      <span className="art-hero-title" aria-hidden="true">
        {showLogo ? (
          <img src={art.logo} alt="" loading="lazy" decoding="async" onError={failLogo} />
        ) : (
          art.text && <span className="art-hero-text text-title">{art.text}</span>
        )}
      </span>
      {backdropFailed && <span className="art-hero-note text-caption">Couldn’t load the backdrop</span>}
    </div>
  )
}
