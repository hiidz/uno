import { matchPath } from 'react-router-dom'

/** The builder's two sections: the profile's own Workspace, and Community. */
export type BuilderSection = 'workspace' | 'community'

/** Where in the builder a path is. */
export interface BuilderView {
  /** One of Nuvio's six profile slots, the range `requireProfile` accepts. */
  profileIndex: number
  section: BuilderSection
}

/** The builder's paths, each under its profile's `/p/{i}`. */
export const builderPath = {
  workspace: (profileIndex: number) => `/p/${profileIndex}`,
  community: (profileIndex: number) => `/p/${profileIndex}/community`,
}

/** The builder view `pathname` is, or null for a path outside the builder or
 *  one naming a slot outside 1–6. */
export function builderView(pathname: string): BuilderView | null {
  const match = matchPath('/p/:profile/*', pathname)
  if (!match) return null
  const profileIndex = profileSlot(match.params.profile)
  if (profileIndex === null) return null
  const rest = match.params['*'] ?? ''
  if (rest === '') return { profileIndex, section: 'workspace' }
  if (rest === 'community') return { profileIndex, section: 'community' }
  return null
}

/** `segment` as a profile slot, 1–6, or null. */
function profileSlot(segment: string | undefined): number | null {
  if (!segment || !/^[1-6]$/.test(segment)) return null
  return Number(segment)
}

/**
 * What moving from `from` to `to` would discard, as the leave guard asks it:
 *
 * - `none`: nothing. The same path, which a history entry of the page's own
 *   (a Home folder page's, the editor layer's) moves between, or a move that
 *   keeps the open editor.
 * - `editor`: the open editor, by moving to the other section.
 * - `builder`: the builder itself, with Home's unpushed edits: a path outside
 *   it, or another profile's.
 */
export type LeaveStep = 'none' | 'editor' | 'builder'

export function leaveStep(from: string, to: string): LeaveStep {
  if (from === to) return 'none'
  const current = builderView(from)
  const next = builderView(to)
  if (!current) return 'none'
  if (!next || next.profileIndex !== current.profileIndex) return 'builder'
  return next.section === current.section ? 'none' : 'editor'
}
