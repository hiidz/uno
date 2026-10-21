import { matchPath } from 'react-router-dom'

/** The builder's two sections: the profile's own Workspace, and Community. */
export type BuilderSection = 'workspace' | 'community'

/** The library row whose editor is open, by its path. */
export interface EditorRef {
  kind: 'catalog' | 'collection'
  id: string
}

/** Where in the builder a path is. */
export interface BuilderView {
  /** One of Nuvio's six profile slots, the range `requireProfile` accepts. */
  profileIndex: number
  section: BuilderSection
  /** The Workspace's open editor; null on Home and in Community. */
  editor: EditorRef | null
  /** Community's open publication page; null on its list and in the
   *  Workspace. */
  publicationID: string | null
}

/** The builder's paths, each under its profile's `/p/{i}`. */
export const builderPath = {
  workspace: (profileIndex: number) => `/p/${profileIndex}`,
  editor: (profileIndex: number, editor: EditorRef) => `/p/${profileIndex}/${editor.kind}/${encodeURIComponent(editor.id)}`,
  community: (profileIndex: number) => `/p/${profileIndex}/community`,
  publication: (profileIndex: number, publicationID: string) =>
    `/p/${profileIndex}/community/${encodeURIComponent(publicationID)}`,
}

/**
 * Marks a history entry the builder pushed over one of its own screens — an
 * editor over Home, a publication's page over Community's list — so closing it
 * steps back to that screen rather than adding another entry; an entry
 * without it, opened from a typed address or a reload, closes by replacing
 * itself.
 */
export const IN_APP = { unoInApp: true }

/** Whether `state` is a location state carrying `IN_APP`. */
export function pushedInApp(state: unknown): boolean {
  return typeof state === 'object' && state !== null && (state as Record<string, unknown>).unoInApp === true
}

/** The builder view `pathname` is, or null for a path outside the builder or
 *  one naming a slot outside 1–6. */
export function builderView(pathname: string): BuilderView | null {
  const match = matchPath('/p/:profile/*', pathname)
  if (!match) return null
  const profileIndex = profileSlot(match.params.profile)
  if (profileIndex === null) return null
  const view = { profileIndex, editor: null, publicationID: null }
  const [first = '', id, ...more] = (match.params['*'] ?? '').split('/')
  if (more.length > 0 || id === '') return null
  if (first === '' && id === undefined) return { ...view, section: 'workspace' }
  if (first === 'community') return { ...view, section: 'community', publicationID: id ?? null }
  if ((first === 'catalog' || first === 'collection') && id !== undefined) {
    return { ...view, section: 'workspace', editor: { kind: first, id } }
  }
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
 *   (a Home folder page's) moves between, a move within the section with no
 *   editor open, or one that keeps the open editor.
 * - `editor`: the open editor, by moving to another editor, Home or the other
 *   section.
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
  if (next.section !== current.section) return 'editor'
  const open = current.editor
  if (!open) return 'none'
  return next.editor?.kind === open.kind && next.editor.id === open.id ? 'none' : 'editor'
}
