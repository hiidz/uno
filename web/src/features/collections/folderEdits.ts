import type { Catalog } from '@/api'
import { formFromCatalog, type CatalogFormState } from '@/features/catalogs/catalogForm'
import {
  hasRef,
  newRef,
  type CollectionErrors,
  type FolderErrors,
  type FolderFormState,
  type FolderRefState,
} from './collectionForm'

/** "Add another genre" on folder `folderKey`'s ref `refKey`: a second ref to
 *  the same catalog, under `genre`, directly below it. Any other folder, an
 *  unknown ref, or a genre that catalog already has here leaves `folder` as
 *  it is. */
export function withGenreRef(folder: FolderFormState, folderKey: string, refKey: string, genre: string): FolderFormState {
  const at = folder.refs.findIndex((ref) => ref.key === refKey)
  if (folder.key !== folderKey || at === -1 || hasRef(folder, folder.refs[at].catalogID, genre)) return folder
  const refs = [...folder.refs]
  refs.splice(at + 1, 0, newRef(folder.refs[at].catalogID, genre))
  return { ...folder, refs }
}

/** Folder `folderKey` with its refs passed through `update`; any other folder
 *  as it is. */
export function withRefs(
  folder: FolderFormState,
  folderKey: string,
  update: (refs: FolderRefState[]) => FolderRefState[],
): FolderFormState {
  return folder.key === folderKey ? { ...folder, refs: update(folder.refs) } : folder
}

/** What the save bar names as needing fixing, in form order: "Title", then
 *  each folder's title and catalogs by its place ("folder 2’s title"). */
export function errorRoleLabels(errors: CollectionErrors, folders: FolderFormState[]): string[] {
  return [...(errors.title ? ['Title'] : []), ...folders.flatMap((folder, index) => folderErrorLabels(errors.folders[folder.key], index))]
}

function folderErrorLabels(folderErrors: FolderErrors | undefined, index: number): string[] {
  return [
    ...(folderErrors?.title ? [`folder ${index + 1}’s title`] : []),
    ...(folderErrors?.catalogIDs ? [`folder ${index + 1}’s catalogs`] : []),
  ]
}

/** The form a collection's nested catalog editor opens on, for the catalog
 *  it has open, if any. */
export function nestedCatalogForm(catalog: Catalog | undefined): CatalogFormState | undefined {
  return catalog && formFromCatalog(catalog)
}
