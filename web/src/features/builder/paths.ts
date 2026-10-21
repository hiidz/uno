/** The builder's two sections: the profile's own Configure section, and Community. */
export type BuilderSection = 'configure' | 'community'

/** A catalog or collection named by a path: the library row whose editor is
 *  open, or the publication whose page is. */
export interface EditorRef {
  kind: 'catalog' | 'collection'
  id: string
}

/** Where in the builder a path is. The profile is not in it: it is the one
 *  picked last (`routes/activeProfile.ts`). */
export interface BuilderView {
  section: BuilderSection
  /** Configure's open editor; null on Home and in Community. */
  editor: EditorRef | null
  /** Community's open publication page; null on its list and in the
   *  Configure. */
  publication: EditorRef | null
}

/** The builder's paths. */
export const builderPath = {
  configure: () => '/configure',
  editor: (editor: EditorRef) => `/configure/${editor.kind}/${encodeURIComponent(editor.id)}`,
  community: () => '/community',
  publication: (publication: EditorRef) => `/community/${publication.kind}/${encodeURIComponent(publication.id)}`,
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

/** The builder view `pathname` is, or null for a path outside the builder. */
export function builderView(pathname: string): BuilderView | null {
  const [, first, kind, id, ...more] = pathname.split('/')
  if (first !== 'configure' && first !== 'community') return null
  const section: BuilderSection = first === 'configure' ? 'configure' : 'community'
  if (!kind && id === undefined) return { section, editor: null, publication: null }
  if ((kind !== 'catalog' && kind !== 'collection') || !id || more.length > 0) return null
  const refID = decoded(id)
  if (refID === null) return null
  const ref: EditorRef = { kind, id: refID }
  if (section === 'configure') return { section, editor: ref, publication: null }
  return { section, editor: null, publication: ref }
}

/** `segment` with its escapes undone, or null for a malformed one. */
function decoded(segment: string): string | null {
  try {
    return decodeURIComponent(segment)
  } catch {
    return null
  }
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
 *   it.
 */
export type LeaveStep = 'none' | 'editor' | 'builder'

export function leaveStep(from: string, to: string): LeaveStep {
  if (from === to) return 'none'
  const current = builderView(from)
  const next = builderView(to)
  if (!current) return 'none'
  if (!next) return 'builder'
  if (next.section !== current.section) return 'editor'
  const open = current.editor
  if (!open) return 'none'
  return next.editor?.kind === open.kind && next.editor.id === open.id ? 'none' : 'editor'
}
