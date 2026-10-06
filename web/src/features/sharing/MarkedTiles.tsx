import { useMemo } from 'react'
import type { Catalog } from '@/api'
import { openFacts, recipeFacts, type RecipeFact } from '@/features/library/recipe'
import type { GenreLookups } from '@/features/library/useLibrary'
import { useRecipeNames } from '@/features/library/useRecipeNames'
import { factKey } from './changeWords'
import { asCatalog } from './snapshot'
import { factChangeNote, recipeChanges, type BlockMark, type FactChange } from './updateMarks'

/**
 * A catalog block's spec tiles as an update waiting on a publication's page
 * marks them: each filter it changes edged in the accent with what it was
 * under its value, and the filters it takes away that no tile shows any more.
 */

/** What a list shows in a tile's value until its names have answered. */
export const NAMES_PENDING = '…'

export const TILE = 'flex min-w-0 flex-col gap-1 rounded-[10px] px-3 py-2.5'

/** The genre lookup for a catalog's own kind: movie and tv ids differ. */
function genresFor(catalog: Catalog, genres: GenreLookups) {
  if (catalog.type === 'movie') return genres.movie
  return genres.tv
}

export function FactValue({ value }: { value: string }) {
  const tone = value === NAMES_PENDING ? 'text-dimmer' : ''
  return <dd className={`m-0 text-[15px] font-bold [overflow-wrap:anywhere] tabular-nums ${tone}`}>{value}</dd>
}


/** A filter the recipe sets, as a tile: a dim label over a bold value, and
 *  while an update changes it, an accent edge with what it was under it. */
export function SetTile({ fact, className, change }: { fact: RecipeFact; className: string; change: FactChange | undefined }) {
  return (
    <div className={`${className} ${changedEdge(change)}`}>
      <dt className="text-dim text-[12px] font-semibold">{fact.label}</dt>
      <FactValue value={fact.value} />
      <ChangeNote change={change} />
    </div>
  )
}

/** A filter the recipe leaves open, as an outlined tile in dimmer type, edged
 *  in the accent instead while an update changes it. */
export function OpenTile({ fact, change }: { fact: RecipeFact; change: FactChange | undefined }) {
  const edge = change ? changedEdge(change) : 'shadow-[inset_0_0_0_1px_var(--uno-line)]'
  return (
    <div className={`${TILE} ${edge}`}>
      <dt className="text-dimmer text-[12px] font-semibold">{fact.label}</dt>
      <dd className="text-dimmer m-0 text-[15px] font-semibold">{fact.value}</dd>
      <ChangeNote change={change} />
    </div>
  )
}

/** Filters an update takes away that no tile shows any more, a left-out list
 *  emptied: an open tile each, so what went is still said. */
export function GoneTiles({ changes, shown }: { changes: ReadonlyMap<string, FactChange>; shown: readonly RecipeFact[] }) {
  const showing = new Set<string>()
  for (const fact of shown) showing.add(factKey(fact.label))
  const gone: [string, FactChange][] = []
  for (const [key, change] of changes) if (!showing.has(key)) gone.push([key, change])
  return gone.map(([key, change]) => <OpenTile key={key} fact={{ label: change.label, value: 'Any' }} change={change} />)
}

function changedEdge(change: FactChange | undefined): string {
  return change ? 'shadow-[inset_0_0_0_1.5px_var(--accent)]' : ''
}

/** What a changed filter was, under its value. */
function ChangeNote({ change }: { change: FactChange | undefined }) {
  if (!change) return null
  return <dd className="text-dimmer m-0 text-[12.5px] [overflow-wrap:anywhere]">{factChangeNote(change)}</dd>
}

export function markID(mark: BlockMark | undefined): string | undefined {
  return mark?.id
}

/** A block the summary jumps to takes focus there without joining the tab
 *  order. */
export function markFocus(mark: BlockMark | undefined): number | undefined {
  return mark?.id ? -1 : undefined
}

export function startsOpen(mark: BlockMark | undefined): boolean {
  return Boolean(mark?.startOpen)
}

export function markWords(mark: BlockMark | undefined): string {
  return mark?.words ?? ''
}

const NO_CHANGES: ReadonlyMap<string, FactChange> = new Map()

/**
 * The filters an update changes on this catalog, by `factKey`, read over
 * every fact each side sets and every filter it leaves open, with the names
 * loaded as `display` says. Empty unless `mark` carries a changed recipe.
 */
export function useMarkedChanges(
  catalog: Catalog,
  mark: BlockMark | undefined,
  genres: GenreLookups,
  display: { foldable: boolean; open: boolean },
): ReadonlyMap<string, FactChange> {
  const was = wasCatalog(mark, catalog)
  const now = useAllFacts(catalog, genres, display)
  const before = useAllFacts(was, genres, display)
  return useMemo(() => changesByKey(was === catalog ? [] : recipeChanges(before, now)), [was, catalog, before, now])
}

/** The catalog as it was before the update, when the update changes its
 *  recipe; the catalog itself otherwise. */
function wasCatalog(mark: BlockMark | undefined, catalog: Catalog): Catalog {
  const old = mark?.recipe?.was_catalog
  return old ? asCatalog(old) : catalog
}

function changesByKey(changes: readonly FactChange[]): ReadonlyMap<string, FactChange> {
  if (changes.length === 0) return NO_CHANGES
  const byKey = new Map<string, FactChange>()
  for (const change of changes) byKey.set(factKey(change.label), change)
  return byKey
}

/** Every fact a recipe sets, named once its lookups answer, then every filter
 *  it leaves open. */
function useAllFacts(catalog: Catalog, genres: GenreLookups, display: { foldable: boolean; open: boolean }): RecipeFact[] {
  const names = useRecipeNames(catalog, display)
  return useMemo(() => {
    const facts = recipeFacts(catalog, genresFor(catalog, genres), names)
    return [...facts, ...openFacts(catalog, facts)]
  }, [catalog, genres, names])
}
