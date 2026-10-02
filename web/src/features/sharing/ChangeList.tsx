import { useState } from 'react'
import type { UseQueryResult } from '@tanstack/react-query'
import type { SnapshotChange } from '@/api'
import type { GenreLookups } from '@/features/library/useLibrary'
import { pluralCount } from '@/lib/plural'
import { changeCount, groupChanges, takeLines, type ChangeGroup } from './changeWords'

/** How many lines show before "and N more". */
const SHOWN = 6

/**
 * A "what changed" list: Removed, Added and Changed, each a dim label over
 * one line for every item, the first six lines, then "and N more", which
 * opens the rest in place. It is the server's order — removals first, since
 * they cost something — in plain ink: no colour marks a removal, because pink
 * means Community and red means destructive.
 */
export function ChangeList({ changes, genres }: { changes: readonly SnapshotChange[]; genres: GenreLookups }) {
  const [expanded, setExpanded] = useState(false)
  const groups = groupChanges(changes, genres)
  const { shown, hidden } = takeLines(groups, expanded ? Infinity : SHOWN)
  return (
    <div className="flex flex-col gap-3">
      {shown.map((group) => (
        <Group key={group.label} group={group} />
      ))}
      {hidden > 0 && (
        <button type="button" onClick={() => setExpanded(true)} className="tap text-dim hover:text-ink self-start text-left text-[13.5px] underline">
          and {hidden} more
        </button>
      )}
    </div>
  )
}

function Group({ group }: { group: ChangeGroup }) {
  return (
    <div className="flex flex-col gap-1">
      <h4 className="text-dim m-0 text-[12px] font-semibold">{group.label}</h4>
      <ul className="m-0 flex list-none flex-col gap-1.5 p-0 text-[14px] leading-snug">
        {group.lines.map((line, index) => (
          <li key={index} className="flex flex-col">
            {line.text}
            {line.notes.map((note) => (
              <span key={note} className="text-dim text-[13px]">
                {note}
              </span>
            ))}
          </li>
        ))}
      </ul>
    </div>
  )
}

const NO_CHANGES: SnapshotChange[] = []

const SECTION = 'flex flex-col gap-2'
const SHELF = `${SECTION} bg-raised rounded-[14px] p-4`

interface ChangesBlockProps {
  title: string
  changes: UseQueryResult<SnapshotChange[]>
  genres: GenreLookups
  shelf?: boolean
}

/**
 * A titled "what changed" list for a call that fetches it: the heading, then
 * the list. Nothing while the call has no changes to show; a quiet line while
 * it loads or if it fails — a list that can't load never stands in the way of
 * the button it sits beside. As a `shelf` it is a raised card with the number
 * of changes at the heading's end.
 */
export function ChangesBlock({ title, changes, genres, shelf }: ChangesBlockProps) {
  const list = changes.data ?? NO_CHANGES
  if (changes.isSuccess && list.length === 0) return null
  return (
    <section aria-label={title} className={sectionClass(shelf)}>
      <ChangesHeading title={title} list={list} shelf={shelf} />
      <ChangesStatus changes={changes} />
      <ChangeList changes={list} genres={genres} />
    </section>
  )
}

function sectionClass(shelf: boolean | undefined): string {
  return shelf ? SHELF : SECTION
}

/** A list's title, with how many changes it holds at the end of a shelf's. */
function ChangesHeading({ title, list, shelf }: { title: string; list: readonly SnapshotChange[]; shelf?: boolean }) {
  const count = shelf ? changeCount(list) : 0
  return (
    <div className="flex items-baseline justify-between gap-3">
      <h3 className="type-label m-0">{title}</h3>
      {count > 0 && <span className="type-data text-dim text-[13px]">{pluralCount(count, 'change')}</span>}
    </div>
  )
}

/** A quiet line while a list loads or when it can't. */
function ChangesStatus({ changes }: { changes: UseQueryResult<SnapshotChange[]> }) {
  return (
    <>
      {changes.isError && <p className="text-dim m-0 text-[13.5px]">Couldn’t load what changed.</p>}
      {changes.isPending && <p className="text-dim m-0 text-[13.5px]">Loading what changed…</p>}
    </>
  )
}
