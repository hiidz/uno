import { Fragment } from 'react'

/** A Community meta line's facts ("Added by 3 · Published 3 weeks ago"),
 *  each kept whole so a narrow column breaks the line between them. */
export function MetaParts({ parts }: { parts: string[] }) {
  return parts.map((part, i) => (
    <Fragment key={part}>
      {i > 0 && ' · '}
      <span className="whitespace-nowrap">{part}</span>
    </Fragment>
  ))
}
