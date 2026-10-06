import type { CatalogPreview, CommunityFolder } from '@/api'

/** How many posters a catalog row fans. */
export const STACK_SIZE = 5

/** How many folder tiles a collection row shows before "+N more". */
export const STRIP_SIZE = 6

/** A catalog row's posters: one per face, the first `STACK_SIZE` results in
 *  order, `''` for a face with no poster yet or none at all. */
export function firstPosters(preview: CatalogPreview | undefined): string[] {
  const items = preview?.items ?? []
  const posters: string[] = []
  for (let i = 0; i < STACK_SIZE; i++) posters.push(items[i]?.poster ?? '')
  return posters
}

/** The folders a collection row draws, and how many it leaves out. */
export interface ShownFolders {
  shown: CommunityFolder[]
  more: number
}

/** A collection row's first `STRIP_SIZE` folders and how many more it has. */
export function shownFolders(folders: CommunityFolder[]): ShownFolders {
  const shown = folders.slice(0, STRIP_SIZE)
  return { shown, more: folders.length - shown.length }
}
