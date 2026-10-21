import type { CommunityItem, SnapshotChange } from '@/api'
import type { FactList, RecipeFact } from '@/features/library/recipe'
import { pluralCount } from '@/lib/plural'
import { factKey } from './changeWords'

/**
 * What an Update changes, placed on the new version it shows: the marks a
 * publication page draws on its folder cards, catalog blocks and tiles, the
 * summary line the In this update shelf reads as, and each changed filter's
 * words. Nothing here is coloured; the page draws the marks in the Community
 * accent.
 */

// ---------- a recipe's filters ----------

/** How a list filter's items changed: what it gained and lost, the join it
 *  had when that changed, and the region it was read in when that did. */
interface ListChange {
  added: string[]
  removed: string[]
  join: 'and' | 'or'
  joinWas: 'and' | 'or' | null
  regionWas: string | null
}

/** One filter that differs: its new value, its old one, and for a list both
 *  sides name, how its items changed. */
export interface FactChange {
  label: string
  value: string
  was: string
  list: ListChange | null
}

const NO_FACT: RecipeFact = { label: '', value: '' }

/**
 * What differs between two recipes, each given as every fact it sets and every
 * filter it leaves open (`recipeFacts` then `openFacts`), so a filter that
 * comes or goes reads against its open value ("was Any"). A singular and a
 * plural label are one filter, named as the new recipe names it.
 */
export function recipeChanges(was: readonly RecipeFact[], now: readonly RecipeFact[]): FactChange[] {
  const before = factsByKey(was)
  const after = factsByKey(now)
  const out: FactChange[] = []
  for (const key of keysOf(after, before)) {
    const was = before.get(key) ?? NO_FACT
    const fact = after.get(key) ?? NO_FACT
    if (fact.value === was.value) continue
    out.push({ label: fact.label || was.label, value: fact.value, was: was.value, list: listChange(was.list, fact.list) })
  }
  return out
}

function factsByKey(facts: readonly RecipeFact[]): Map<string, RecipeFact> {
  const byKey = new Map<string, RecipeFact>()
  for (const fact of facts) byKey.set(factKey(fact.label), fact)
  return byKey
}

/** Every key in `first`, then the ones only `second` has. */
function keysOf(first: ReadonlyMap<string, RecipeFact>, second: ReadonlyMap<string, RecipeFact>): string[] {
  const keys = [...first.keys()]
  for (const key of second.keys()) if (!first.has(key)) keys.push(key)
  return keys
}

/** How a list's items changed, when both sides name them. */
function listChange(before: FactList | undefined, after: FactList | undefined): ListChange | null {
  if (!before || !after) return null
  return {
    added: itemsBeyond(after.items, before.items),
    removed: itemsBeyond(before.items, after.items),
    join: after.join,
    // A join change a reader can see: both sides hold more than one item.
    joinWas: before.join !== after.join && before.items.length > 1 && after.items.length > 1 ? before.join : null,
    regionWas: before.region === after.region ? null : before.region,
  }
}

/** The items of `have` that `other` lacks, in order. */
function itemsBeyond(have: readonly string[], other: readonly string[]): string[] {
  const beyond: string[] = []
  for (const item of have) if (!other.includes(item)) beyond.push(item)
  return beyond
}

const JOIN_WORDS = { and: 'all of them', or: 'any of them' } as const

/**
 * The words under a changed filter's new value: for a list, what it gained
 * and lost, its join and its region ("added War, Western · now matches any of
 * them, was all of them · was in United States"); otherwise its old value
 * ("was 6.5–9.5", "was Any" for a filter that was not set).
 */
export function factChangeNote(change: FactChange): string {
  const list = change.list
  if (!list) return `was ${change.was || 'Any'}`
  const parts: string[] = []
  if (list.added.length) parts.push(`added ${list.added.join(', ')}`)
  if (list.removed.length) parts.push(`removed ${list.removed.join(', ')}`)
  if (list.joinWas) parts.push(`now matches ${JOIN_WORDS[list.join]}, was ${JOIN_WORDS[list.joinWas]}`)
  if (list.regionWas !== null) parts.push(list.regionWas ? `was in ${list.regionWas}` : 'was anywhere')
  return parts.length > 0 ? parts.join(' · ') : `was ${change.was || 'Any'}`
}

// ---------- marks on the new version ----------

/** What happened to a folder the new version holds. */
export interface FolderMark {
  added: boolean
  was: string
  art: boolean
  catalogOrder: boolean
}

/** A catalog whose name or recipe changed: its old name when renamed, and
 *  the change carrying both recipes when its recipe changed. */
interface CatalogMark {
  was: string
  recipe: SnapshotChange | null
}

/** Everything an update changes, by where the new version shows it. */
export interface UpdateMarks {
  folders: Map<string, FolderMark>
  /** `folderKey/catalogKey` of each catalog added to a folder. */
  added: Set<string>
  /** The catalogs removed from each folder the new version keeps, by name. */
  removed: Map<string, string[]>
  /** The folders the new version drops, by title. */
  removedFolders: string[]
  catalogs: Map<string, CatalogMark>
  collection: { was: string; settings: boolean; order: boolean }
}

const NO_FOLDER_MARK: FolderMark = { added: false, was: '', art: false, catalogOrder: false }

function emptyMarks(): UpdateMarks {
  return {
    folders: new Map(),
    added: new Set(),
    removed: new Map(),
    removedFolders: [],
    catalogs: new Map(),
    collection: { was: '', settings: false, order: false },
  }
}

/** Places each change of an update on what the new version shows. A catalog
 *  removed with its whole folder goes with the folder. */
export function updateMarks(changes: readonly SnapshotChange[]): UpdateMarks {
  const marks = emptyMarks()
  const goneFolders = new Set<string>()
  for (const change of changes) {
    const key = change.key ?? ''
    switch (`${change.kind}/${change.op}`) {
      case 'folder/added':
        marks.folders.set(key, { ...folderMark(marks, key), added: true })
        break
      case 'folder/removed':
        marks.removedFolders.push(change.name ?? '')
        goneFolders.add(key)
        break
      case 'folder/changed': {
        const changed = changedFolder(folderMark(marks, key), change)
        if (changed) marks.folders.set(key, changed)
        break
      }
      case 'catalog/added':
        if (change.folder_key) marks.added.add(`${change.folder_key}/${change.key}`)
        break
      case 'catalog/removed': {
        const folder = change.folder_key ?? ''
        if (folder && !goneFolders.has(folder)) {
          marks.removed.set(folder, [...(marks.removed.get(folder) ?? []), catalogLabel(change)])
        }
        break
      }
      case 'catalog/changed':
        marks.catalogs.set(key, { was: change.was ?? '', recipe: change.aspect === 'recipe' ? change : null })
        break
      case 'collection/changed':
        markCollection(marks, change)
        break
    }
  }
  return marks
}

function folderMark(marks: UpdateMarks, key: string): FolderMark {
  return marks.folders.get(key) ?? NO_FOLDER_MARK
}

/** `mark` with the aspect a changed folder names; null for an aspect a
 *  folder card doesn't show. */
function changedFolder(mark: FolderMark, change: SnapshotChange): FolderMark | null {
  if (change.aspect === 'name') return { ...mark, was: change.was ?? '' }
  if (change.aspect === 'art') return { ...mark, art: true }
  if (change.aspect === 'catalog_order') return { ...mark, catalogOrder: true }
  return null
}

/** A catalog's name, with the genre a folder narrows it to. */
function catalogLabel(change: SnapshotChange): string {
  const name = change.name ?? ''
  return change.genre ? `${name} (${change.genre})` : name
}

function markCollection(marks: UpdateMarks, change: SnapshotChange): void {
  const collection = marks.collection
  if (change.aspect === 'name') collection.was = change.was ?? ''
  if (change.aspect === 'settings') collection.settings = true
  if (change.aspect === 'order') collection.order = true
}

/** What a folder's mark says: "new", "renamed · was 80s", "new art",
 *  "catalog order changed"; '' for a folder the update leaves as it was. */
export function folderMarkWords(mark: FolderMark | undefined): string {
  const folder = mark ?? NO_FOLDER_MARK
  if (folder.added) return 'new'
  const parts: string[] = []
  if (folder.was) parts.push(`renamed · was ${folder.was}`)
  if (folder.art) parts.push('new art')
  if (folder.catalogOrder) parts.push('catalog order changed')
  return parts.join(' · ')
}

/** What a catalog's mark in a folder says. Its full marks (the tiles) show
 *  where the new version first uses it; elsewhere it says so in words. */
export function catalogMarkWords(mark: CatalogMark | undefined, added: boolean, first: boolean): string {
  const catalog = mark ?? NO_CATALOG_MARK
  const parts = added ? ['added'] : []
  // The old name where its full marks show; "renamed" alone anywhere else.
  if (catalog.was) parts.push(first ? `renamed · was ${catalog.was}` : 'renamed')
  if (catalog.recipe) parts.push(first ? 'filters changed' : 'filters changed, see above')
  return parts.join(' · ')
}

const NO_CATALOG_MARK: CatalogMark = { was: '', recipe: null }

/** `folderKey/index` of the first ref to each catalog, in the new version's
 *  folder order: where a changed catalog shows its marked tiles. */
export function firstUses(folders: readonly SnapshotFolderLike[]): Set<string> {
  const seen = new Set<string>()
  const first = new Set<string>()
  for (const folder of folders) {
    const refs = folder.refs ?? []
    for (let index = 0; index < refs.length; index++) {
      const catalog = refs[index]!.catalog_id
      if (seen.has(catalog)) continue
      seen.add(catalog)
      first.add(`${folder.id}/${index}`)
    }
  }
  return first
}

/** A folder as the publication page reads it: its key as its id, and its
 *  refs by catalog key. */
export interface SnapshotFolderLike {
  id: string
  refs?: { catalog_id: string }[] | null
}

// ---------- the summary line ----------

/** One part of the summary line, and the id of the mark it jumps to; none
 *  for a change no card shows. */
export interface SummaryItem {
  text: string
  target: string | null
}

/** The id of a folder card the summary jumps to. */
export function folderMarkID(key: string): string {
  return `update-folder-${key}`
}

/** The id of the block where a changed catalog shows its marks. */
function catalogMarkID(key: string): string {
  return `update-catalog-${key}`
}

/** The id of the card listing the folders an update removes. */
export const REMOVED_ID = 'update-removed'

/** The id of the line saying what a renamed publication was called. */
export const RENAMED_ID = 'update-renamed'

/**
 * The In this update shelf's one line: what the update does, counted, each
 * part jumping to where the new version shows it — "1 new folder · 2 folders
 * renamed · 1 folder removed · 1 catalog changed · folder order changed". A
 * catalog publication's reads "renamed · filters changed".
 */
export function updateSummary(marks: UpdateMarks, kind: CommunityItem['kind']): SummaryItem[] {
  const items: SummaryItem[] = []
  const renamed = renamedFrom(marks, kind)
  if (renamed) items.push({ text: `renamed, was ${renamed}`, target: RENAMED_ID })
  if (kind === 'catalog') return [...items, ...catalogSummary(marks)]
  return [...items, ...folderSummary(marks), ...catalogsSummary(marks), ...collectionSummary(marks)]
}

/** What the publication itself was called, when the update renames it: the
 *  collection, or a catalog publication's one catalog. */
export function renamedFrom(marks: UpdateMarks, kind: CommunityItem['kind']): string {
  if (kind !== 'catalog') return marks.collection.was
  const [only] = [...marks.catalogs.values()]
  return only ? only.was : ''
}

function catalogSummary(marks: UpdateMarks): SummaryItem[] {
  const [entry] = [...marks.catalogs.entries()]
  if (!entry?.[1].recipe) return []
  return [{ text: 'filters changed', target: catalogMarkID(entry[0]) }]
}

/** The folders whose mark passes `test`, counted as one summary part that
 *  jumps to the first. */
function folderPart(marks: UpdateMarks, test: (mark: FolderMark) => boolean, word: (count: number) => string): SummaryItem[] {
  const keys: string[] = []
  for (const [key, mark] of marks.folders) if (test(mark)) keys.push(key)
  if (keys.length === 0) return []
  return [{ text: word(keys.length), target: folderMarkID(keys[0]!) }]
}

function folderSummary(marks: UpdateMarks): SummaryItem[] {
  return [
    ...folderPart(marks, (mark) => mark.added, (n) => pluralCount(n, 'new folder')),
    ...folderPart(marks, (mark) => !mark.added && Boolean(mark.was), (n) => `${pluralCount(n, 'folder')} renamed`),
    ...folderPart(marks, (mark) => !mark.added && mark.art, (n) => `new art on ${pluralCount(n, 'folder')}`),
    ...folderPart(marks, (mark) => !mark.added && mark.catalogOrder, (n) => `catalog order changed in ${pluralCount(n, 'folder')}`),
    ...(marks.removedFolders.length ? [{ text: `${pluralCount(marks.removedFolders.length, 'folder')} removed`, target: REMOVED_ID }] : []),
  ]
}

/** Catalogs added to and removed from folders the new version keeps, and the
 *  catalogs whose name or recipe changed. */
function catalogsSummary(marks: UpdateMarks): SummaryItem[] {
  const items: SummaryItem[] = []
  const addedTo = keptFolderAdds(marks)
  if (addedTo.length) items.push({ text: `${pluralCount(addedTo.length, 'catalog')} added`, target: folderMarkID(addedTo[0]!) })
  const removedFrom = [...marks.removed.keys()]
  const removedCount = [...marks.removed.values()].reduce((sum, names) => sum + names.length, 0)
  if (removedCount) items.push({ text: `${pluralCount(removedCount, 'catalog')} removed`, target: folderMarkID(removedFrom[0]!) })
  const changed = [...marks.catalogs.keys()]
  if (changed.length) items.push({ text: `${pluralCount(changed.length, 'catalog')} changed`, target: catalogMarkID(changed[0]!) })
  return items
}

/** The folder of each catalog added to a folder that is not itself new. */
function keptFolderAdds(marks: UpdateMarks): string[] {
  const folders: string[] = []
  for (const entry of marks.added) {
    const folder = entry.slice(0, entry.indexOf('/'))
    if (!folderMark(marks, folder).added) folders.push(folder)
  }
  return folders
}

function collectionSummary(marks: UpdateMarks): SummaryItem[] {
  const items: SummaryItem[] = []
  if (marks.collection.order) items.push({ text: 'folder order changed', target: null })
  if (marks.collection.settings) items.push({ text: 'view settings changed', target: null })
  return items
}

// ---------- marks on a catalog block ----------

/** What a catalog block draws of an update: its words, the change carrying
 *  both recipes when its tiles are marked, the id the summary jumps to, and
 *  whether it starts open so its marked tiles show. */
export interface BlockMark {
  words: string
  recipe: SnapshotChange | null
  id: string | undefined
  startOpen: boolean
}

/**
 * The mark on a catalog in a folder card: added, renamed, its filters
 * changed. Where the new version first uses a changed catalog (`first`) the
 * block starts open on its marked tiles and is what the summary jumps to;
 * elsewhere it says so in words. Undefined while no update waits.
 */
export function folderEntryMark(
  marks: UpdateMarks | null,
  folderKey: string,
  catalogKey: string,
  first: boolean,
): BlockMark | undefined {
  if (!marks) return undefined
  const change = marks.catalogs.get(catalogKey)
  const words = catalogMarkWords(change, marks.added.has(`${folderKey}/${catalogKey}`), first)
  if (!first || !change) return { words, recipe: null, id: undefined, startOpen: false }
  return { words, recipe: change.recipe, id: catalogMarkID(catalogKey), startOpen: Boolean(change.recipe) }
}

/** The mark on a catalog publication's own tiles: its changed filters, with
 *  no words, since the summary says them. */
export function pageBlockMark(marks: UpdateMarks | null, catalogKey: string): BlockMark | undefined {
  const change = marks?.catalogs.get(catalogKey)
  if (!change) return undefined
  return { words: '', recipe: change.recipe, id: catalogMarkID(catalogKey), startOpen: true }
}
