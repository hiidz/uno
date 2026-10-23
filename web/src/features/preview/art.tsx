import { useState } from 'react'
import { Check, Copy, ImageOff } from 'lucide-react'
import type { TileShape } from '@/api'
import { Icon } from '@/components/Icon'
import { useCopy } from '@/features/push/useCopy'
import { prefersReducedMotion } from '@/lib/motion'
import { cutAddress, shapeClass, siteOf, type HeroArt } from './heroArt'

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

/** What a left-out box is drawn at: a tile shape, a background image (16:9,
 *  narrower on a phone) or a title logo (Nuvio's 220 × 100 logo box). */
export type LeftOutShape = TileShape | 'BACKDROP' | 'LOGO'

/**
 * A picture the server left out of Community, in its place: a dashed box at
 * the picture's shape saying so, with its address cut to fit and an icon that
 * copies the whole of it. The address is never loaded. A narrow box (poster,
 * square, a phone's background image) names the site alone. A tap on the
 * address opens it whole, wrapped, the box growing to hold it; another cuts it
 * again.
 */
export function LeftOutPicture({ url, shape }: { url: string; shape: LeftOutShape }) {
  const { copied, copy } = useCopy(url)
  const [open, setOpen] = useState(false)
  const site = siteOf(url) || url
  const narrow = shape === 'POSTER' || shape === 'SQUARE'
  return (
    <span className={`art-pic art-out ${leftOutClass(shape)}${open ? ' is-open' : ''}`}>
      <span className="art-out-note text-caption">
        <Icon icon={ImageOff} size={16} />
        <span>Left out</span>
        <span className="art-out-addr">
          {/* drift-ok: the address inside a left-out box, a toggle drawn by `.art-out-url` */}
          <button type="button" className="art-out-url" title={url} aria-expanded={open} onClick={() => setOpen(!open)}>
            {open ? url : narrow ? site : <AddressCut url={url} site={site} backdrop={shape === 'BACKDROP'} />}
          </button>
          {/* drift-ok: the copy icon inside a left-out box, drawn by `.art-out-copy` */}
          <button type="button" className="art-out-copy" aria-label="Copy address" title="Copy address" onClick={copy}>
            <Icon icon={copied ? Check : Copy} size={13} />
          </button>
        </span>
      </span>
    </span>
  )
}

/** The address cut to its box; a background image's, narrower on a phone,
 *  names the site alone there. */
function AddressCut({ url, site, backdrop }: { url: string; site: string; backdrop: boolean }) {
  const cut = cutAddress(url, 24)
  if (!backdrop) return cut
  return (
    <>
      <span className="art-out-wide">{cut}</span>
      <span className="art-out-narrow">{site}</span>
    </>
  )
}

function leftOutClass(shape: LeftOutShape): string {
  if (shape === 'BACKDROP') return 'backdrop'
  if (shape === 'LOGO') return 'logo'
  return shapeClass(shape)
}
