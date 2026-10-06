import type { Catalog } from '@/api'
import { formFromCatalog, type CatalogFormState } from '@/features/catalogs/catalogForm'
import {
  flatRefs,
  hasRef,
  newRef,
  refGroups,
  type RefGroup,
  type CollectionErrors,
  type FolderErrors,
  type FolderFormState,
  type FolderRefState,
} from './collectionForm'
import type { RefOption } from './refs'

/** Folder `folderKey` with its refs passed through `update`; any other folder
 *  as it is. */
export function withRefs(
  folder: FolderFormState,
  folderKey: string,
  update: (refs: FolderRefState[]) => FolderRefState[],
): FolderFormState {
  return folder.key === folderKey ? { ...folder, refs: update(folder.refs) } : folder
}

/** Folder `folderKey` with `update` applied; any other folder as it is. */
export function withFolderUpdate(
  folder: FolderFormState,
  folderKey: string,
  update: Partial<FolderFormState>,
): FolderFormState {
  return folder.key === folderKey ? { ...folder, ...update } : folder
}

/** True when `refs` hold `catalogID` under any genre: the Add catalogs
 *  dropdown's tick. */
export function holdsCatalog(refs: FolderRefState[], catalogID: string): boolean {
  return refs.some((ref) => ref.catalogID === catalogID)
}

/** The catalog ids of `refs`, once each, in folder order. */
export function catalogOrder(refs: FolderRefState[]): string[] {
  const ids: string[] = []
  for (const group of refGroups(refs)) ids.push(group.catalogID)
  return ids
}

/** `refs` with whole catalogs in `catalogIDs`' order, each catalog's own refs
 *  kept in theirs. A catalog not listed keeps its place after the listed
 *  ones. */
export function withCatalogOrder(refs: FolderRefState[], catalogIDs: string[]): FolderRefState[] {
  const groups = refGroups(refs)
  const ordered: RefGroup[] = []
  for (const id of catalogIDs) {
    const at = groups.findIndex((group) => group.catalogID === id)
    if (at !== -1) ordered.push(...groups.splice(at, 1))
  }
  return flatRefs([...ordered, ...groups])
}

/** `refs` with `catalogID`'s own refs in `refKeys`' order, the catalog
 *  keeping its place. A key not listed stays after the listed ones. */
export function withGenreOrder(refs: FolderRefState[], catalogID: string, refKeys: string[]): FolderRefState[] {
  const groups = refGroups(refs)
  for (const group of groups) {
    if (group.catalogID === catalogID) group.refs = byKeys(group.refs, refKeys)
  }
  return flatRefs(groups)
}

function byKeys(refs: FolderRefState[], keys: string[]): FolderRefState[] {
  const rest = [...refs]
  const ordered: FolderRefState[] = []
  for (const key of keys) {
    const at = rest.findIndex((ref) => ref.key === key)
    if (at !== -1) ordered.push(...rest.splice(at, 1))
  }
  return [...ordered, ...rest]
}

/** `refs` with one more of `catalogID`'s tabs, under `genre` (`''` for no
 *  genre filter), after that catalog's last. A catalog the folder doesn't
 *  hold, or a genre it already has here, leaves `refs` as they are: the same
 *  catalog under the same genre twice is a pair `folder_catalogs`' primary
 *  key forbids. */
export function withGenreAdded(refs: FolderRefState[], catalogID: string, genre: string): FolderRefState[] {
  if (hasRef(refs, catalogID, genre)) return refs
  const groups = refGroups(refs)
  const group = groups.find((g) => g.catalogID === catalogID)
  if (!group) return refs
  group.refs.push(newRef(catalogID, genre))
  return flatRefs(groups)
}

/** `refs` without ref `refKey`, unless it is its catalog's only one: a
 *  catalog leaves the folder whole, by Remove from folder or the Add
 *  catalogs untick. */
export function withoutRef(refs: FolderRefState[], refKey: string): FolderRefState[] {
  const ref = refs.find((r) => r.key === refKey)
  if (!ref || refs.filter((r) => r.catalogID === ref.catalogID).length < 2) return refs
  return refs.filter((r) => r.key !== refKey)
}

/** `refs` without any of `catalogID`'s. */
export function withoutCatalog(refs: FolderRefState[], catalogID: string): FolderRefState[] {
  return refs.filter((ref) => ref.catalogID !== catalogID)
}

/** `refs` with an unfiltered ref to `catalogID` at the end, unless they
 *  hold that catalog under any genre already. */
export function withCatalogAdded(refs: FolderRefState[], catalogID: string): FolderRefState[] {
  if (holdsCatalog(refs, catalogID)) return refs
  return [...refs, newRef(catalogID)]
}

/** `refs` with every ref to `from` pointed at `to`, each keeping its place
 *  and genre. */
export function withCatalogMoved(refs: FolderRefState[], from: string, to: string): FolderRefState[] {
  return refs.map((ref) => (ref.catalogID === from ? { ...ref, catalogID: to } : ref))
}

/** True for a catalog held once with no genre filter: its line offers
 *  "Split by genre" rather than genre chips. */
export function isUnsplit(refs: FolderRefState[]): boolean {
  return refs.length === 1 && refs[0].genre === ''
}

/** One line of a catalog's genre dropdown. */
export interface GenreChoice {
  /** `''` for no genre filter. */
  genre: string
  /** The ref under this genre, when the folder has one. */
  refKey: string | undefined
  /** Stored here, but the recipe's genre options no longer include it. */
  stale: boolean
}

/**
 * What a catalog's genre dropdown offers: no genre filter, then the recipe's
 * own genre options (`offered`, undefined until they land), then any genre
 * the folder holds that they don't, so a stored value always has a line to
 * untick. Such a genre is stale only once the options have landed: the addon
 * path then serves it unfiltered.
 */
export function genreChoices(offered: string[] | undefined, refs: FolderRefState[]): GenreChoice[] {
  const choices: GenreChoice[] = []
  for (const genre of choiceGenres(offered, refs)) {
    choices.push({ genre, refKey: refKeyFor(refs, genre), stale: isStale(offered, genre) })
  }
  return choices
}

function choiceGenres(offered: string[] | undefined, refs: FolderRefState[]): string[] {
  const genres = ['', ...(offered ?? [])]
  for (const ref of refs) {
    if (!genres.includes(ref.genre)) genres.push(ref.genre)
  }
  return genres
}

function refKeyFor(refs: FolderRefState[], genre: string): string | undefined {
  for (const ref of refs) {
    if (ref.genre === genre) return ref.key
  }
  return undefined
}

function isStale(offered: string[] | undefined, genre: string): boolean {
  if (offered === undefined || genre === '') return false
  return !offered.includes(genre)
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

/** The catalogs `ids` name in `optionByID`, in the order given; an id it
 *  doesn't hold is skipped. */
export function catalogsOf(ids: string[], optionByID: ReadonlyMap<string, RefOption>): Catalog[] {
  const catalogs: Catalog[] = []
  for (const id of ids) {
    const option = optionByID.get(id)
    if (option) catalogs.push(option.catalog)
  }
  return catalogs
}

/** A folder list item's name, for the drag announcements: a catalog by its
 *  id, or one of its genres by ref `key` ("Hidden Gems, Horror"). */
export function dragItemName(folders: FolderFormState[], optionByID: ReadonlyMap<string, RefOption>, id: string): string {
  const ref = refByKey(folders, id)
  if (!ref) return catalogName(optionByID, id)
  const name = catalogName(optionByID, ref.catalogID)
  if (ref.genre === '') return name
  return `${name}, ${ref.genre}`
}

function refByKey(folders: FolderFormState[], key: string): FolderRefState | undefined {
  for (const folder of folders) {
    for (const ref of folder.refs) {
      if (ref.key === key) return ref
    }
  }
  return undefined
}

function catalogName(optionByID: ReadonlyMap<string, RefOption>, catalogID: string): string {
  return optionByID.get(catalogID)?.name ?? 'this catalog'
}
