import { useCallback, useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import type { Library } from '@/features/library/useLibrary'
import { IN_APP, builderPath, pushedInApp, type EditorRef } from './paths'
import { catalogTarget, collectionTarget, type EditorTarget } from './target'

interface EditorRoute {
  /** The open editor, seeded from its row as it was when it opened. */
  target: EditorTarget | null
  /** The path names a row this profile's library doesn't hold. */
  missing: boolean
  /** Opens `next`'s editor: over Home a step Back returns from, in place of
   *  another open editor without one. */
  open: (next: EditorTarget) => void
  /** Closes the open editor, back to Home. */
  close: () => void
}

/**
 * The Workspace's open editor, as its path names it (`paths.ts`).
 *
 * The target is caught once per row, as the path first names it: from what
 * `open` was handed, which is a row the library may not list yet (just
 * created or duplicated), or else from the library's row. It is not
 * re-derived from later reads, so `initial.revision` stays the revision the
 * editor opened at and a save over a newer one is refused.
 *
 * Every open and close is a navigation, so `LeaveGuard` asks before it
 * discards unsaved changes. `onSwitch` runs as the editor changes, once the
 * new one has rendered.
 */
export function useEditorRoute(
  profileIndex: number,
  editor: EditorRef | null,
  library: Pick<Library, 'catalogs' | 'collections' | 'loaded'>,
  onSwitch: () => void,
): EditorRoute {
  const navigate = useNavigate()
  const location = useLocation()
  const [handedOver, setHandedOver] = useState<EditorTarget | null>(null)
  const [caught, setCaught] = useState<EditorTarget | null>(null)

  const current = editor && caught && sameRow(caught, editor) ? caught : null
  const found = editor && !current ? targetFor(editor, handedOver, library) : null
  if (!editor && caught) setCaught(null)
  if (found) setCaught(found)
  // Caught, so a later visit to the same row reads it afresh.
  if (current && handedOver) setHandedOver(null)

  const key = editor && `${editor.kind}/${editor.id}`
  const switched = useRef(onSwitch)
  useEffect(() => {
    switched.current = onSwitch
  })
  useEffect(() => {
    switched.current()
  }, [key])

  const pushedState = useRef(location.state)
  useEffect(() => {
    pushedState.current = location.state
  })

  const open = useCallback(
    (next: EditorTarget) => {
      setHandedOver(next)
      const path = builderPath.editor(profileIndex, next)
      if (editor) void navigate(path, { replace: true, state: pushedState.current })
      else void navigate(path, { state: IN_APP })
    },
    [navigate, profileIndex, editor],
  )

  const close = useCallback(() => {
    if (pushedInApp(pushedState.current)) void navigate(-1)
    else void navigate(builderPath.workspace(profileIndex), { replace: true })
  }, [navigate, profileIndex])

  const missing = editor !== null && current === null && found === null && library.loaded
  return { target: current, missing, open, close }
}

function sameRow(target: EditorTarget, editor: EditorRef): boolean {
  return target.kind === editor.kind && target.id === editor.id
}

/** `editor`'s target: the one handed to `open`, or else its library row's. */
function targetFor(
  editor: EditorRef,
  handed: EditorTarget | null,
  library: Pick<Library, 'catalogs' | 'collections'>,
): EditorTarget | null {
  if (handed && sameRow(handed, editor)) return handed
  if (editor.kind === 'catalog') {
    const row = library.catalogs.find((catalog) => catalog.id === editor.id)
    return row ? catalogTarget(row) : null
  }
  const row = library.collections.find((collection) => collection.id === editor.id)
  return row ? collectionTarget(row) : null
}
