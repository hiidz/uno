import { Fragment } from 'react'
import { REMOVED_ID, RENAMED_ID, type SummaryItem } from './updateMarks'

/**
 * The pieces a publication page draws an update's marks with, in the
 * Community accent: the In this update shelf's summary line, the words on a
 * folder card or catalog block, the catalogs a folder loses and the folders
 * the update removes.
 */

/** Scrolls to the mark `id` names and puts focus there. */
function jumpTo(id: string) {
  const target = document.getElementById(id)
  if (!target) return
  target.scrollIntoView({ block: 'center', behavior: reducedMotion() ? 'auto' : 'smooth' })
  target.focus({ preventScroll: true })
}

/** Whether the viewer asks for less motion; a scroll then jumps. */
function reducedMotion(): boolean {
  return window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
}

/** The shelf's one line: each part that the new version shows somewhere a
 *  link that scrolls to it, the rest plain words. */
export function UpdateSummary({ items }: { items: readonly SummaryItem[] }) {
  return (
    <p className="m-0 text-[14px] leading-[1.7] font-semibold">
      {items.map((item, index) => (
        <Fragment key={item.text}>
          {index > 0 && <span className="text-dim font-normal"> · </span>}
          <SummaryPart item={item} />
        </Fragment>
      ))}
    </p>
  )
}

function SummaryPart({ item }: { item: SummaryItem }) {
  const { target } = item
  if (!target) return <span>{item.text}</span>
  return (
    <button
      type="button"
      onClick={() => jumpTo(target)}
      className="cursor-pointer text-left underline decoration-[var(--accent)] decoration-[1.5px] underline-offset-[3px] hover:text-[var(--accent)]"
    >
      {item.text}
    </button>
  )
}

/** A mark's words in the accent, under what it marks; nothing when there are
 *  none. */
export function MarkLine({ words, className = '' }: { words: string; className?: string }) {
  if (!words) return null
  return <p className={`m-0 text-[12.5px] font-semibold [overflow-wrap:anywhere] text-[var(--accent)] ${className}`}>{words}</p>
}

/** The catalogs an update removes from a folder the new version keeps, dim at
 *  the foot of its card. */
export function RemovedCatalogs({ names }: { names: readonly string[] }) {
  return names.map((name) => (
    <p key={name} className="border-line text-dimmer m-0 flex justify-between gap-3 border-t py-2 pl-[26px] text-[14px]">
      <span className="min-w-0 [overflow-wrap:anywhere]">{name}</span>
      <span className="shrink-0 text-[12.5px] text-[var(--accent)]">removed</span>
    </p>
  ))
}

/** The folders an update removes, after the last folder card: outlined, with
 *  no fill, since the new version holds none of them. */
export function RemovedFolders({ names }: { names: readonly string[] }) {
  if (names.length === 0) return null
  return (
    <div id={REMOVED_ID} tabIndex={-1} className="flex flex-col gap-1 rounded-[16px] px-5 py-3 shadow-[inset_0_0_0_1px_var(--uno-line-hi)] outline-none">
      {names.map((name) => (
        <p key={name} className="text-dimmer m-0 flex justify-between gap-3 text-[15px] font-semibold">
          <span className="min-w-0 [overflow-wrap:anywhere]">{name}</span>
          <span className="shrink-0 text-[12.5px] text-[var(--accent)]">folder removed</span>
        </p>
      ))}
    </div>
  )
}

/** What a renamed publication was called, dim under its meta line, where the
 *  summary's "renamed" jumps to; nothing when the update keeps its name. */
export function RenamedFrom({ name }: { name: string }) {
  if (!name) return null
  return (
    <p id={RENAMED_ID} tabIndex={-1} className="text-dimmer m-0 text-[12.5px] [overflow-wrap:anywhere] outline-none">
      was {name}
    </p>
  )
}
