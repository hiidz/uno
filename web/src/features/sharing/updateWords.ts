import type { SnapshotChange, SnapshotFolder } from '@/api'
import type { RecipeFact } from '@/features/library/recipe'
import { pluralCount } from '@/lib/plural'
import { factKey } from './changeWords'

/**
 * What an Update changes, by folder, in the words of the In this update
 * shelf: each folder the update touches under the name the new version gives
 * it, in the new version's folder order, with what happens to the folder
 * itself ("renamed · was 80s", "new folder · 4 catalogs") and, under it, each
 * catalog that changes with what happens to it at the line's end ("removed",
 * "changed"). A catalog that goes or comes with its whole folder is the
 * folder's count, not a line of its own. Nothing here is coloured.
 */

/** One catalog an update touches, or a change to the collection itself. */
export interface UpdateRow {
  change: SnapshotChange
  name: string
  status: string
}

/** One folder an update touches; `folder` is '' for the collection itself and
 *  for a catalog published on its own. */
export interface UpdateSection {
  folder: string
  notes: string[]
  rows: UpdateRow[]
}

/** The new name of each folder the update renames, by its old one. */
function renames(changes: readonly SnapshotChange[]): Map<string, string> {
  const renamed = changes.filter((c) => c.kind === 'folder' && c.aspect === 'name' && c.was)
  return new Map(renamed.map((c) => [c.was ?? '', c.name ?? '']))
}

/** `op/name` of each folder removed or added whole. */
function wholeFolders(changes: readonly SnapshotChange[]): Set<string> {
  return new Set(changes.filter((c) => c.kind === 'folder' && c.op !== 'changed').map((c) => `${c.op}/${c.name}`))
}

/** Whether a catalog goes or comes with its whole folder. */
function goesWithFolder(change: SnapshotChange, whole: ReadonlySet<string>): boolean {
  return whole.has(`${change.op}/${change.folder}`)
}

/** The first folder of the new version that holds the catalog `key`. */
function folderHolding(folders: readonly SnapshotFolder[], key: string | undefined): string {
  const holder = folders.find((folder) => (folder.refs ?? []).some((ref) => ref.catalog === key))
  return holder?.title ?? ''
}

/** The folder a catalog's change belongs under: the one it leaves or joins,
 *  else the one the new version holds it in. */
function catalogFolder(change: SnapshotChange, folders: readonly SnapshotFolder[]): string {
  return change.folder ?? folderHolding(folders, change.catalog?.key)
}

/** A whole folder's note, with the catalogs that go or come with it. */
function withCatalogs(note: string, catalogs: number): string {
  return catalogs ? `${note} · ${pluralCount(catalogs, 'catalog')}` : note
}

const FOLDER_NOTE: Record<string, (change: SnapshotChange, catalogs: number) => string> = {
  'removed/': (_, catalogs) => withCatalogs('folder removed', catalogs),
  'added/': (_, catalogs) => withCatalogs('new folder', catalogs),
  'changed/name': (change) => `renamed · was ${change.was}`,
  'changed/art': () => 'new art',
  'changed/catalog_order': () => 'new catalog order',
}

const COLLECTION_ROW: Record<string, (change: SnapshotChange) => Omit<UpdateRow, 'change'>> = {
  name: (change) => ({ name: change.name ?? '', status: `renamed · was ${change.was}` }),
  settings: () => ({ name: 'Collection settings', status: 'changed' }),
  order: () => ({ name: 'Folder order', status: 'changed' }),
}

/** What happens to a catalog, at its line's end. */
function catalogStatus(change: SnapshotChange): string {
  return change.aspect === 'name' ? `renamed · was ${change.was}` : change.op
}

/** A catalog's line name, with the genre a folder narrows it to. */
function catalogName(change: SnapshotChange): string {
  return change.genre ? `${change.name} (${change.genre})` : (change.name ?? '')
}

/** Where each folder sits in the new version; the collection's own changes
 *  first, folders the new version drops last. */
function rank(folders: readonly SnapshotFolder[], folder: string): number {
  if (!folder) return -1
  const at = folders.findIndex((f) => f.title === folder)
  return at === -1 ? folders.length : at
}

/** A comparison's items as folders, in the new version's folder order. */
export function groupByFolder(changes: readonly SnapshotChange[], folders: readonly SnapshotFolder[] = []): UpdateSection[] {
  const renamed = renames(changes)
  const whole = wholeFolders(changes)
  const sections = new Map<string, UpdateSection>()
  const at = (name: string): UpdateSection => {
    const folder = renamed.get(name) ?? name
    const section = sections.get(folder) ?? { folder, notes: [], rows: [] }
    sections.set(folder, section)
    return section
  }
  const place: Record<SnapshotChange['kind'], (change: SnapshotChange) => void> = {
    collection: (change) => {
      const row = COLLECTION_ROW[change.aspect ?? '']
      if (row) at('').rows.push({ change, ...row(change) })
    },
    folder: (change) => {
      const note = FOLDER_NOTE[`${change.op}/${change.op === 'changed' ? change.aspect : ''}`]
      const catalogs = changes.filter((c) => c.kind === 'catalog' && c.op === change.op && c.folder === change.name)
      if (note) at(change.name ?? '').notes.push(note(change, catalogs.length))
    },
    catalog: (change) => {
      if (goesWithFolder(change, whole)) return
      at(catalogFolder(change, folders)).rows.push({ change, name: catalogName(change), status: catalogStatus(change) })
    },
  }
  for (const change of changes) place[change.kind](change)
  return [...sections.values()].sort((a, b) => rank(folders, a.folder) - rank(folders, b.folder))
}

/** The lines a section draws: its folder's own change, then its catalogs. */
function sectionLines(section: UpdateSection): number {
  return (section.notes.length ? 1 : 0) + section.rows.length
}

/** The first `limit` lines of `sections`, in their folders, and how many
 *  lines that leaves out. */
export function takeRows(
  sections: readonly UpdateSection[],
  limit: number,
): { shown: UpdateSection[]; hidden: number } {
  let left = limit
  const shown = sections.flatMap((section) => {
    if (left < 1) return []
    left -= section.notes.length ? 1 : 0
    const rows = section.rows.slice(0, Math.max(0, left))
    left -= rows.length
    return [{ ...section, rows }]
  })
  const count = (list: readonly UpdateSection[]) => list.reduce((sum, section) => sum + sectionLines(section), 0)
  return { shown, hidden: count(sections) - count(shown) }
}

/** One filter whose value an update changes: the new value and the old. */
export interface FactChange {
  label: string
  value: string
  was: string
}

/**
 * What differs between two recipes, each given as every fact it sets and every
 * filter it leaves open (`recipeFacts` then `openFacts`), so a filter that
 * comes or goes reads against its open value: "Studio Ghibli or Pixar · was
 * Any", "Highest rated · was Most popular". A singular and a plural label are
 * one filter, named as the new recipe names it.
 */
export function recipeChanges(was: readonly RecipeFact[], now: readonly RecipeFact[]): FactChange[] {
  const before = new Map(was.map((fact) => [factKey(fact.label), fact]))
  const after = new Map(now.map((fact) => [factKey(fact.label), fact]))
  const keys = [...new Set([...after.keys(), ...before.keys()])]
  return keys.flatMap((key) => {
    const next = after.get(key)
    const prev = before.get(key)
    if (next?.value === prev?.value) return []
    return [{ label: (next ?? prev)?.label ?? '', value: next?.value ?? '', was: prev?.value ?? '' }]
  })
}
