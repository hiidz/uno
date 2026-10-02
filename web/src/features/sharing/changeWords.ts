import type { Catalog, SnapshotChange } from '@/api'
import { recipeFacts, type RecipeFact } from '@/features/library/recipe'
import type { GenreLookups } from '@/features/library/useLibrary'
import { pluralCount } from '@/lib/plural'
import { asCatalog } from './snapshot'

/**
 * A server comparison's items (`SnapshotChange`) in the words a person reads:
 * lines under Removed, Added and Changed, one for each thing that changed,
 * with no verb repeated beside its group's label. The server orders the items
 * — removals, then additions, then changes — and these keep that order;
 * nothing here is coloured, since pink means Community and red means
 * destructive.
 */

/** One thing that changed: its line, and under it what changed in it. */
interface ChangeLine {
  text: string
  notes: string[]
}

export interface ChangeGroup {
  label: 'Removed' | 'Added' | 'Changed'
  lines: ChangeLine[]
}

const GROUP_LABEL: Record<SnapshotChange['op'], ChangeGroup['label']> = {
  removed: 'Removed',
  added: 'Added',
  changed: 'Changed',
}

/** A comparison item, with the number of catalogs its folder line absorbed. */
interface Item {
  change: SnapshotChange
  catalogs: number
}

/** Whether an item is a folder removed or added whole. */
function isWholeFolder({ op, kind }: SnapshotChange): boolean {
  return kind === 'folder' && (op === 'removed' || op === 'added')
}

/** The key an item answers to in its folder's line: `op/folder`, the folder
 *  being the item itself when it is one. */
function folderKey(change: SnapshotChange): string {
  return `${change.op}/${change.kind === 'folder' ? change.name : change.folder}`
}

/** Whether a catalog is removed from or added to a folder that is removed or
 *  added whole, so its folder's line counts it. */
function isFolded(change: SnapshotChange, whole: ReadonlySet<string>): boolean {
  return change.kind === 'catalog' && whole.has(folderKey(change))
}

/** How many catalogs each whole folder's line counts, by `folderKey`. */
function countFolded(changes: readonly SnapshotChange[], whole: ReadonlySet<string>): Map<string, number> {
  const counts = new Map<string, number>()
  for (const change of changes.filter((c) => isFolded(c, whole))) {
    counts.set(folderKey(change), (counts.get(folderKey(change)) ?? 0) + 1)
  }
  return counts
}

/** The items that have words, one line each: a catalog removed from or added
 *  to a folder that is itself removed or added is part of that folder's line,
 *  as its count. */
function collapse(changes: readonly SnapshotChange[]): Item[] {
  const whole = new Set(changes.filter(isWholeFolder).map(folderKey))
  const counts = countFolded(changes, whole)
  return changes
    .filter((change) => WORDS[wordsKey(change)] && !isFolded(change, whole))
    .map((change) => ({ change, catalogs: counts.get(folderKey(change)) ?? 0 }))
}

/** How many lines a comparison draws: what the update changes, counted. */
export function changeCount(changes: readonly SnapshotChange[] = []): number {
  return collapse(changes).length
}

function quoted(name: string | undefined): string {
  return `“${name ?? ''}”`
}

/** "“A” → “B”", or just the new one while the old is unknown. */
function renamed(was: string | undefined, name: string | undefined): string {
  return was ? `${quoted(was)} → ${quoted(name)}` : quoted(name)
}

/** A folder removed or added whole, with the catalogs that went with it. */
function folderLine({ change, catalogs }: Item): ChangeLine {
  const count = catalogs ? ` · ${pluralCount(catalogs, 'catalog')}` : ''
  return { text: `Folder ${quoted(change.name)}${count}`, notes: [] }
}

/** A catalog with the genre a folder narrows it to after it, and the folder
 *  named by `joiner`. */
function catalogLine({ change }: Item, joiner: 'from' | 'to'): ChangeLine {
  const genre = change.genre ? ` (${change.genre})` : ''
  const folder = change.folder ? ` ${joiner} ${quoted(change.folder)}` : ''
  return { text: `${quoted(change.name)}${genre}${folder}`, notes: [] }
}

/** A fact's label as one key for its singular and its plural: "Studio" and
 *  "Studios" are one fact. */
function factKey(label: string): string {
  return label.toLowerCase().replace(/s$/, '')
}

/** A list recipeFacts could not name, which it counts. */
function isCount(value: string): boolean {
  return /^\d+$/.test(value)
}

/** A value, or "any" for a fact the recipe does not set. */
function orAny(value: string | undefined): string {
  return value ?? 'any'
}

/** A fact that came or went: "Keyword added" for a list counted, else its
 *  value against "any". */
function appearance(label: string, before: string | undefined, after: string | undefined): string {
  const gone = after === undefined
  const value = gone ? before : after
  if (value && isCount(value)) return `${label} ${gone ? 'removed' : 'added'}`
  return `${label}: ${orAny(before)} → ${orAny(after)}`
}

/** A fact that appears, disappears or changes value, as the line saying so. */
function factNote(label: string, before: string | undefined, after: string | undefined): string {
  if (before === after) return ''
  if (before === undefined || after === undefined) return appearance(label, before, after)
  return `${label}: ${before} → ${after}`
}

/** A recipe's facts by `factKey`. */
function factsOf(catalog: Catalog, genres: GenreLookups): Map<string, RecipeFact> {
  const lookup = catalog.type === 'movie' ? genres.movie : genres.tv
  return new Map(recipeFacts(catalog, lookup).map((fact) => [factKey(fact.label), fact]))
}

/** The longer of two facts' labels, the plural of a singular and a plural. */
function longerLabel(before: RecipeFact | undefined, after: RecipeFact | undefined): string {
  const labels = [before?.label, after?.label].flatMap((label) => label ?? [])
  return labels.reduce((longest, label) => (label.length > longest.length ? label : longest), '')
}

/** What differs between two recipes' facts, one line each: the plural label
 *  where the two differ in number, a list the lookups have not named as its
 *  count. */
function recipeNotes(was: Catalog, now: Catalog, genres: GenreLookups): string[] {
  const before = factsOf(was, genres)
  const after = factsOf(now, genres)
  const keys = new Set([...after.keys(), ...before.keys()])
  return [...keys]
    .map((key) => factNote(longerLabel(before.get(key), after.get(key)), before.get(key)?.value, after.get(key)?.value))
    .filter(Boolean)
}

/** A catalog whose recipe changed: its name, and what changed in it. */
function recipeLine({ change }: Item, genres: GenreLookups): ChangeLine {
  const name = change.was ? [`Name: ${renamed(change.was, change.name)}`] : []
  const filters =
    change.was_catalog && change.catalog
      ? recipeNotes(asCatalog(change.was_catalog), asCatalog(change.catalog), genres)
      : []
  return { text: quoted(change.name), notes: [...name, ...(filters.length ? filters : ['Filters changed'])] }
}

type Words = (item: Item, genres: GenreLookups) => ChangeLine

function line(text: string): ChangeLine {
  return { text, notes: [] }
}

/** Each kind of item as a line, by `op/kind`, and `op/kind/aspect` for a
 *  change. */
const WORDS: Record<string, Words> = {
  'removed/folder': folderLine,
  'removed/catalog': (item) => catalogLine(item, 'from'),
  'added/folder': folderLine,
  'added/catalog': (item) => catalogLine(item, 'to'),
  'changed/collection/name': ({ change }) => line(`Collection name: ${renamed(change.was, change.name)}`),
  'changed/collection/settings': () => line('Collection settings'),
  'changed/collection/order': () => line('Folder order'),
  'changed/folder/name': ({ change }) => line(`Folder name: ${renamed(change.was, change.name)}`),
  'changed/folder/art': ({ change }) => line(`Art on ${quoted(change.name)}`),
  'changed/folder/catalog_order': ({ change }) => line(`Catalog order in ${quoted(change.name)}`),
  'changed/catalog/name': ({ change }) => ({
    text: quoted(change.name),
    notes: [`Name: ${renamed(change.was, change.name)}`],
  }),
  'changed/catalog/recipe': recipeLine,
}

/** The key `WORDS` words an item under. */
function wordsKey({ op, kind, aspect }: SnapshotChange): string {
  return [op, kind, op === 'changed' ? aspect : undefined].filter(Boolean).join('/')
}

/**
 * A comparison as its groups: Removed, Added, Changed, each a line for every
 * item, in the server's order. A catalog removed from or added to a folder that
 * is itself removed or added is not a line of its own: the folder's line
 * counts it.
 */
export function groupChanges(changes: readonly SnapshotChange[], genres: GenreLookups): ChangeGroup[] {
  const groups = new Map<ChangeGroup['label'], ChangeLine[]>()
  for (const item of collapse(changes)) {
    const words = WORDS[wordsKey(item.change)]
    const label = GROUP_LABEL[item.change.op]
    groups.set(label, [...(groups.get(label) ?? []), words(item, genres)])
  }
  return [...groups].map(([label, lines]) => ({ label, lines }))
}

/** The first `limit` lines of `groups`, in their groups, and how many lines
 *  that leaves out. */
export function takeLines(groups: readonly ChangeGroup[], limit: number): { shown: ChangeGroup[]; hidden: number } {
  let left = limit
  const shown = groups.flatMap((group) => {
    const lines = group.lines.slice(0, Math.max(0, left))
    left -= lines.length
    return lines.length ? [{ ...group, lines }] : []
  })
  const count = (list: readonly ChangeGroup[]) => list.reduce((sum, group) => sum + group.lines.length, 0)
  return { shown, hidden: count(groups) - count(shown) }
}
