import type { PreviewCollection, PreviewFolder } from '@/features/preview/model'

/** How many folders a Home row draws before the rest become "+N more". */
const SHOWN = 6

/** A Home row's summary line under its name: a catalog's recipe, or a
 *  collection that has no folders to show. */
export function DetailLine({ text }: { text: string }) {
  return <span className="text-dim col-[2/-1] truncate text-[13.5px]">{text}</span>
}

/** A collection row's folders as tiles, or its summary while it has none
 *  ("0 folders") or nothing describes it any more. */
export function CollectionDetail({ collection, summary }: { collection: PreviewCollection; summary: string }) {
  if (collection.folders.length === 0) return <DetailLine text={summary} />
  return <FolderChips folders={collection.folders} />
}

/**
 * A collection row's folders on Home, in its own order: each a small tile
 * with its cover art (or emoji) and its title, the first six of them, then a
 * dim "+N more".
 */
function FolderChips({ folders }: { folders: PreviewFolder[] }) {
  const hidden = folders.length - SHOWN
  return (
    <ul aria-label="Folders" className="col-[2/-1] m-0 flex list-none flex-wrap items-center gap-1.5 p-0">
      {folders.slice(0, SHOWN).map((folder) => (
        <FolderChip key={folder.id} folder={folder} />
      ))}
      {hidden > 0 && <li className="text-dimmer px-1 text-[12.5px]">+{hidden} more</li>}
    </ul>
  )
}

/** One folder's tile: its face, then its title. A folder with neither cover
 *  image nor emoji is its title alone. */
function FolderChip({ folder }: { folder: PreviewFolder }) {
  const hasFace = Boolean(folder.coverImageUrl || folder.coverEmoji)
  return (
    <li className={`bg-raised flex h-7 max-w-[11rem] items-center gap-1.5 rounded-lg py-0.5 pr-2.5 ${hasFace ? 'pl-0.5' : 'pl-2.5'}`}>
      {hasFace && <FolderFace folder={folder} />}
      <span className="text-dim truncate text-[12.5px] font-semibold">{folder.title || 'Untitled folder'}</span>
    </li>
  )
}

/** A folder's cover in a 24px square: its image, else its emoji. */
function FolderFace({ folder }: { folder: PreviewFolder }) {
  return (
    <span
      aria-hidden="true"
      className="bg-raised-hi grid size-6 shrink-0 place-items-center overflow-hidden rounded-md text-[13px] shadow-[inset_0_0_0_1px_var(--uno-line-hi)]"
    >
      <FaceArt folder={folder} />
    </span>
  )
}

function FaceArt({ folder }: { folder: PreviewFolder }) {
  if (folder.coverImageUrl) return <img src={folder.coverImageUrl} alt="" loading="lazy" className="size-full object-cover" />
  return folder.coverEmoji
}
