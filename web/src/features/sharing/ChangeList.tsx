import { useState, type ReactNode } from 'react'
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
        // drift-ok: an underlined text link that shows the rest of the list
        <button type="button" onClick={() => setExpanded(true)} className="tap text-dim hover:text-ink text-meta self-start text-left underline">
          and {hidden} more
        </button>
      )}
    </div>
  )
}

function Group({ group }: { group: ChangeGroup }) {
  return (
    <div className="flex flex-col gap-1">
      <h4 className="text-dim text-caption m-0 font-semibold">{group.label}</h4>
      <ul className="text-body-sm m-0 flex list-none flex-col gap-1.5 p-0 leading-snug">
        {group.lines.map((line, index) => (
          <li key={index} className="flex flex-col">
            {line.text}
            {line.notes.map((note) => (
              <span key={note} className="text-dim text-meta">
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
const SHELF = 'flex flex-col gap-3.5 bg-raised-hi rounded-[16px] px-[18px] pt-4 pb-[18px]'

interface ChangesBlockProps {
  title: string
  changes: UseQueryResult<SnapshotChange[]>
  genres: GenreLookups
  shelf?: boolean
  /** The list drawn under the heading; `ChangeList` unless given. */
  renderList?: (changes: SnapshotChange[], genres: GenreLookups) => ReactNode
}

function changeList(changes: SnapshotChange[], genres: GenreLookups): ReactNode {
  return <ChangeList changes={changes} genres={genres} />
}

/**
 * A titled "what changed" list for a call that fetches it: the heading, then
 * the list (`renderList`). Nothing while the call has no changes to show; a
 * quiet line while it loads or if it fails — a list that can't load never
 * stands in the way of the button it sits beside. As a `shelf` it is a
 * raised-hi card, a step above the folder cards beside it, its heading in sign
 * lettering in the region's accent with the number of changes at the heading's
 * end.
 */
export function ChangesBlock({ title, changes, genres, shelf, renderList = changeList }: ChangesBlockProps) {
  const list = changes.data ?? NO_CHANGES
  if (changes.isSuccess && list.length === 0) return null
  return (
    <section aria-label={title} className={sectionClass(shelf)}>
      <ChangesHeading title={title} list={list} shelf={shelf} />
      <ChangesStatus changes={changes} />
      {renderList(list, genres)}
    </section>
  )
}

function sectionClass(shelf: boolean | undefined): string {
  return shelf ? SHELF : SECTION
}

/** A shelf's heading is sign lettering in the region's accent; a plain list's
 *  is a label. */
function headingClass(shelf: boolean | undefined): string {
  return shelf ? 'type-sign text-sign m-0 text-[var(--accent)]' : 'type-label m-0'
}

/** A list's title, with how many changes it holds at the end of a shelf's. */
function ChangesHeading({ title, list, shelf }: { title: string; list: readonly SnapshotChange[]; shelf?: boolean }) {
  const count = shelf ? changeCount(list) : 0
  return (
    <div className="flex items-baseline justify-between gap-3">
      <h3 className={headingClass(shelf)}>{title}</h3>
      {count > 0 && <span className="type-data text-dim text-meta">{pluralCount(count, 'change')}</span>}
    </div>
  )
}

/** A quiet line while a list loads or when it can't. */
function ChangesStatus({ changes }: { changes: UseQueryResult<SnapshotChange[]> }) {
  return (
    <>
      {changes.isError && <p className="text-dim text-meta m-0">Couldn’t load what changed.</p>}
      {changes.isPending && <p className="text-dim text-meta m-0">Loading what changed…</p>}
    </>
  )
}
