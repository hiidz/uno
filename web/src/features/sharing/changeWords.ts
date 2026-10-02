import type { SnapshotChange } from '@/api'
import type { GenreLookups } from '@/features/library/useLibrary'
import { pluralCount } from '@/lib/plural'
import { snapshotRecipeLine } from './snapshot'

/**
 * A server comparison's items (`SnapshotChange`) in the words a person reads:
 * the lines of a "what changed" list, and the one summary line under Update….
 * The server orders the items — removals, then additions, then changes — and
 * these keep that order; nothing here is coloured, since pink means Community
 * and red means destructive.
 */

function quoted(name: string | undefined): string {
  return `“${name ?? ''}”`
}

/** A catalog's name, with the genre a folder narrows it to after it. */
function catalogName(change: SnapshotChange): string {
  return change.genre ? `${quoted(change.name)} (${change.genre})` : quoted(change.name)
}

function removedCatalog(change: SnapshotChange): string {
  return change.folder ? `Removed ${catalogName(change)} from ${quoted(change.folder)}` : `Removed ${catalogName(change)}`
}

function addedCatalog(change: SnapshotChange): string {
  return change.folder ? `Added ${catalogName(change)} to ${quoted(change.folder)}` : `Added ${catalogName(change)}`
}

/** A catalog whose recipe changed, with its new recipe line — and its earlier
 *  name when that changed too. */
function changedRecipe(change: SnapshotChange, genres: GenreLookups): string {
  const was = change.was ? ` (was ${quoted(change.was)})` : ''
  const line = change.catalog ? snapshotRecipeLine(change.catalog, genres) : ''
  return `${quoted(change.name)}${was} now: ${line || 'no filters'}`
}

type Words = (change: SnapshotChange, genres: GenreLookups) => string

/** Each kind of item as a line, by `op/kind`, and `op/kind/aspect` for a
 *  change. */
const WORDS: Record<string, Words> = {
  'removed/folder': (change) => `Removed folder ${quoted(change.name)}`,
  'removed/catalog': removedCatalog,
  'added/folder': (change) => `Added folder ${quoted(change.name)}`,
  'added/catalog': addedCatalog,
  'changed/collection/name': (change) => `Renamed the collection to ${quoted(change.name)}`,
  'changed/collection/settings': () => 'Collection settings changed',
  'changed/collection/order': () => 'Folder order changed',
  'changed/folder/name': (change) => `Renamed folder ${quoted(change.was)} to ${quoted(change.name)}`,
  'changed/folder/art': (change) => `Art changed on ${quoted(change.name)}`,
  'changed/folder/catalog_order': (change) => `Catalog order changed in ${quoted(change.name)}`,
  'changed/catalog/name': (change) => `Renamed ${quoted(change.was)} to ${quoted(change.name)}`,
  'changed/catalog/recipe': changedRecipe,
}

/** The key `WORDS` words a change under. */
function wordsKey(change: SnapshotChange): string {
  return [change.op, change.kind, change.op === 'changed' ? change.aspect : undefined].filter(Boolean).join('/')
}

/** One item as a line: "Removed “Retro” from “80s”", "“Horror” now: Highest
 *  rated · Horror". `genres` words a changed recipe. */
export function changeWords(change: SnapshotChange, genres: GenreLookups): string {
  return (WORDS[wordsKey(change)] ?? (() => ''))(change, genres)
}

/** The summary's parts in order, by what they count: the item's `op/kind`,
 *  with every change but a catalog's counted as `other`. */
const SUMMARY: ReadonlyArray<readonly [key: string, noun: string, verb: string]> = [
  ['removed/folder', 'folder', 'removed'],
  ['removed/catalog', 'catalog', 'removed'],
  ['added/folder', 'folder', 'added'],
  ['added/catalog', 'catalog', 'added'],
  ['changed/catalog', 'catalog', 'changed'],
]

function summaryKey(change: SnapshotChange): string {
  return change.op === 'changed' && change.kind !== 'catalog' ? 'other' : `${change.op}/${change.kind}`
}

/** What a list holds in one line, in the list's own order, for the From
 *  Community view: "1 folder removed · 2 catalogs added · 1 catalog changed".
 *  Empty for no changes. */
export function changeSummary(changes: readonly SnapshotChange[]): string {
  const counts = new Map<string, number>()
  for (const change of changes) counts.set(summaryKey(change), (counts.get(summaryKey(change)) ?? 0) + 1)
  const parts = SUMMARY.flatMap(([key, noun, verb]) => {
    const count = counts.get(key)
    return count ? [`${pluralCount(count, noun)} ${verb}`] : []
  })
  const other = counts.get('other')
  if (other) parts.push(pluralCount(other, parts.length ? 'other change' : 'change'))
  return parts.join(' · ')
}
