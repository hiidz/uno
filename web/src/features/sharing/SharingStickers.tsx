import type { SharingSticker } from './sharingState'
import { stickerClass } from './sharingState'

/** A row's stickers and flags (`sharingState.ts`), in their own words. */
export function SharingStickers({ stickers }: { stickers: SharingSticker[] }) {
  return (
    <>
      {stickers.map((sticker) => (
        <span key={sticker.label} className={stickerClass(sticker.tone)}>
          {sticker.label}
        </span>
      ))}
    </>
  )
}
