import { useState } from 'react'
import type { Catalog, SnapshotChange, SnapshotFolder } from '@/api'
import { openFacts, recipeFacts, type RecipeFact } from '@/features/library/recipe'
import type { GenreLookups } from '@/features/library/useLibrary'
import { useRecipeNames } from '@/features/library/useRecipeNames'
import { asCatalog } from './snapshot'
import { groupByFolder, recipeChanges, takeRows, type FactChange, type UpdateRow, type UpdateSection } from './updateWords'

/** How many lines show before "and N more". */
const SHOWN = 6

/**
 * The In this update shelf's list: each folder the update touches, its name
 * as the new version has it with what happens to the folder at the right,
 * ruled off from the catalogs in it that change, each with what happens to it
 * at its line's end; a changed recipe lists each filter's new value with the
 * old one dim after "was". `folders` is the new version's, which orders the
 * list and places a changed catalog. The first six lines show, then "and N
 * more" opens the rest in place.
 */
export function UpdateList({
  changes,
  folders,
  genres,
}: {
  changes: readonly SnapshotChange[]
  folders: readonly SnapshotFolder[]
  genres: GenreLookups
}) {
  const [expanded, setExpanded] = useState(false)
  const { shown, hidden } = takeRows(groupByFolder(changes, folders), expanded ? Infinity : SHOWN)
  return (
    <div className="flex flex-col gap-4">
      {shown.map((section) => (
        <Section key={section.folder} section={section} genres={genres} />
      ))}
      {hidden > 0 && (
        <button type="button" onClick={() => setExpanded(true)} className="tap text-dim hover:text-ink self-start text-left text-[13.5px] underline">
          and {hidden} more
        </button>
      )}
    </div>
  )
}

function Section({ section, genres }: { section: UpdateSection; genres: GenreLookups }) {
  return (
    <div className="flex flex-col gap-2">
      {section.folder && <FolderHead section={section} />}
      {section.rows.map((row, index) => (
        <Row key={`${row.change.op}/${row.name}/${index}`} row={row} genres={genres} />
      ))}
    </div>
  )
}

/** A folder heading is ruled off from the catalogs under it, when it has any. */
function headClass(section: UpdateSection): string {
  return section.rows.length ? 'border-line-hi border-b pb-1.5' : ''
}

function FolderHead({ section }: { section: UpdateSection }) {
  return (
    <div className={`flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5 ${headClass(section)}`}>
      <span className="min-w-0 flex-[1_1_12rem] text-[14.5px] font-bold [overflow-wrap:anywhere]">{section.folder}</span>
      <span className="text-dim ml-auto min-w-0 text-right text-[12.5px] [overflow-wrap:anywhere]">{section.notes.join(' · ')}</span>
    </div>
  )
}

function Row({ row, genres }: { row: UpdateRow; genres: GenreLookups }) {
  const { change } = row
  return (
    <div>
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5">
        <span className="min-w-0 flex-[1_1_12rem] text-[14px] [overflow-wrap:anywhere]">{row.name}</span>
        <span className="text-dim ml-auto shrink-0 text-right text-[12.5px]">{row.status}</span>
      </div>
      {change.aspect === 'recipe' && change.catalog && change.was_catalog && (
        <RecipeChanges was={asCatalog(change.was_catalog)} now={asCatalog(change.catalog)} genres={genres} />
      )}
    </div>
  )
}

/** Every fact a recipe sets, named once its lookups answer, then every filter
 *  it leaves open. */
function useAllFacts(catalog: Catalog, genres: GenreLookups): RecipeFact[] {
  const names = useRecipeNames(catalog, { foldable: false, open: true })
  const facts = recipeFacts(catalog, catalog.type === 'movie' ? genres.movie : genres.tv, names)
  return [...facts, ...openFacts(catalog, facts)]
}

/** A changed recipe's filters, the new value bold and the old dim after
 *  "was"; "Filters changed" when nothing a tile shows differs. */
function RecipeChanges({ was, now, genres }: { was: Catalog; now: Catalog; genres: GenreLookups }) {
  const changed = recipeChanges(useAllFacts(was, genres), useAllFacts(now, genres))
  if (changed.length === 0) return <p className="text-dim m-0 mt-1 pl-3 text-[13px]">Filters changed</p>
  return (
    <div className="fact-changes mt-1.5 pl-3">
      <dl className="m-0">
        {changed.map((fact) => (
          <FactLine key={fact.label} fact={fact} />
        ))}
      </dl>
    </div>
  )
}

function FactLine({ fact }: { fact: FactChange }) {
  return (
    <div className="fact-line">
      <dt className="text-dim text-[13px]">{fact.label}</dt>
      <dd className="m-0 text-[13px]">
        <span className="mr-1.5 font-semibold [overflow-wrap:anywhere] empty:hidden">{fact.value}</span>
        <span className="text-dimmer [overflow-wrap:anywhere]">was {fact.was}</span>
      </dd>
    </div>
  )
}
