import { STICKER_CLASS, type SharingSticker } from './sharingState'

/** One row of a `CatalogList`: a catalog's name over its recipe line, with
 *  an optional sticker, a quiet kind sticker unless `tone` says otherwise. */
export interface CatalogListItem {
  key: string
  name: string
  line: string
  sticker?: string
  tone?: SharingSticker['tone']
}

/** Catalogs by name over their recipe line, the way the rail lists them: the
 *  publish dialog's lists. */
export function CatalogList({ items }: { items: CatalogListItem[] }) {
  return (
    <ul className="m-0 list-none p-0">
      {items.map((item) => (
        <li key={item.key} className="flex min-w-0 flex-col gap-0.5 py-1.5">
          <span className="flex items-center gap-2 text-[15px] font-semibold">
            {item.name}
            <ItemSticker item={item} />
          </span>
          <span className="text-dim text-[13px]">{item.line}</span>
        </li>
      ))}
    </ul>
  )
}

function ItemSticker({ item }: { item: CatalogListItem }) {
  if (!item.sticker) return null
  return <span className={STICKER_CLASS[item.tone ?? 'quiet']}>{item.sticker}</span>
}
