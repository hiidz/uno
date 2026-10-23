import type { ReactNode } from 'react'
import { FocusGIFTile, HeroPreview, PictureTile } from '@/features/preview/art'
import { heroArt } from '@/features/preview/heroArt'
import type { FolderFormState } from './collectionForm'
import type { ShownPictures } from './useShownPictures'

/** A picture under its field. Each one below draws nothing until its address
 *  is one to show. */
function FieldPicture({ children }: { children: ReactNode }) {
  return <div className="pt-1">{children}</div>
}

/** A picture field's line saying Community leaves it out (`withheldNote`),
 *  or the shelf's saying Community shares no pictures here; nothing without
 *  `text`. */
export function CommunityNote({ text }: { text: string | undefined }) {
  if (!text) return null
  return <p className="ed-note m-0">{text}</p>
}

export function CoverPicture({ src, folder }: { src: string; folder: FolderFormState }) {
  if (!src) return null
  return (
    <FieldPicture>
      <PictureTile src={src} shape={folder.tileShape} />
    </FieldPicture>
  )
}

/** The focus GIF, only while the folder plays it. */
export function FocusGIFPicture({ src, folder }: { src: string; folder: FolderFormState }) {
  if (!src || !folder.focusGIFEnabled) return null
  return (
    <FieldPicture>
      <FocusGIFTile src={src} shape={folder.tileShape} />
    </FieldPicture>
  )
}

/** The collection's background image, as a landscape tile: it fills each
 *  coverless folder's tile and sits behind a hero with no backdrop. */
export function BackgroundPicture({ src }: { src: string }) {
  if (!src) return null
  return (
    <FieldPicture>
      <PictureTile src={src} shape="LANDSCAPE" />
    </FieldPicture>
  )
}

export function LogoPicture({ src }: { src: string }) {
  if (!src) return null
  return (
    <FieldPicture>
      <PictureTile src={src} />
    </FieldPicture>
  )
}

/**
 * The folder's Modern Home hero, once its hero backdrop or title logo is
 * filled: the backdrop falls back to the cover and the collection's
 * background image, as Nuvio TV's does.
 */
export function FolderHero({
  folder,
  shown,
  backdropImageURL,
}: {
  folder: FolderFormState
  shown: ShownPictures
  /** The collection's background image, settled. */
  backdropImageURL: string
}) {
  if (!shown.heroBackdrop && !shown.titleLogo) return null
  const art = heroArt({
    title: folder.title,
    hideTitle: folder.hideTitle,
    coverEmoji: folder.coverEmoji,
    coverImageURL: shown.cover,
    heroBackdropURL: shown.heroBackdrop,
    titleLogoURL: shown.titleLogo,
    backdropImageURL,
  })
  return (
    <div className="setting">
      <span className="setting-label type-label">Hero preview</span>
      <div className="setting-value flex max-w-[360px] flex-col gap-2">
        <HeroPreview art={art} />
      </div>
    </div>
  )
}
