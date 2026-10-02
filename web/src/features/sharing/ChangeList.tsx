import { useState } from 'react'
import type { UseQueryResult } from '@tanstack/react-query'
import type { SnapshotChange } from '@/api'
import type { GenreLookups } from '@/features/library/useLibrary'
import { changeWords } from './changeWords'

/** How many lines show before "and N more". */
const SHOWN = 5

/**
 * A "what changed" list: one line for each item, the first five, then "and N
 * more", which opens the rest in place. It is the server's order — removals
 * first, since they cost something — in plain ink: no colour marks a removal,
 * because pink means Community and red means destructive.
 */
export function ChangeList({ changes, genres }: { changes: readonly SnapshotChange[]; genres: GenreLookups }) {
  const [expanded, setExpanded] = useState(false)
  const shown = expanded ? changes : changes.slice(0, SHOWN)
  const hidden = changes.length - shown.length
  return (
    <div className="flex flex-col gap-1.5">
      <ul className="marker:text-dimmer m-0 flex list-disc flex-col gap-1 pl-5 text-[14px] leading-snug">
        {shown.map((change, index) => (
          <li key={index}>{changeWords(change, genres)}</li>
        ))}
      </ul>
      {hidden > 0 && (
        <button type="button" onClick={() => setExpanded(true)} className="tap text-dim hover:text-ink self-start text-left text-[13.5px] underline">
          and {hidden} more
        </button>
      )}
    </div>
  )
}

/**
 * A titled "what changed" list for a call that fetches it: the heading, then
 * the list. Nothing while the call has no changes to show; a quiet line while
 * it loads or if it fails — a list that can't load never stands in the way of
 * the button it sits beside.
 */
export function ChangesBlock({
  title,
  changes,
  genres,
}: {
  title: string
  changes: UseQueryResult<SnapshotChange[]>
  genres: GenreLookups
}) {
  const list = changes.data ?? []
  if (changes.isSuccess && list.length === 0) return null
  return (
    <section aria-label={title} className="flex flex-col gap-2">
      <h3 className="type-label m-0">{title}</h3>
      {changes.isError && <p className="text-dim m-0 text-[13.5px]">Couldn’t load what changed.</p>}
      {changes.isPending && <p className="text-dim m-0 text-[13.5px]">Loading what changed…</p>}
      <ChangeList changes={list} genres={genres} />
    </section>
  )
}
