import { useCallback, useMemo, useState } from 'react'
import { Navigate } from 'react-router-dom'
import { ProfileNotSelectedError } from '@/api'
import type { CatalogType } from '@/api'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { CatalogEditor } from '@/features/catalogs/CatalogEditor'
import { NewCatalogDialog } from '@/features/catalogs/NewCatalogDialog'
import {
  emptyForm,
  formFromCatalog,
  toPayload,
  type CatalogFormState,
} from '@/features/catalogs/catalogForm'
import { useCatalogMutations } from '@/features/catalogs/useCatalogMutations'
import { CollectionEditor } from '@/features/collections/CollectionEditor'
import { NewCollectionDialog } from '@/features/collections/NewCollectionDialog'
import {
  emptyCollectionForm,
  formFromCollection,
  toCollectionPayload,
  type CollectionFormState,
} from '@/features/collections/collectionForm'
import { accessibleIDs, buildRefOptions, indexRefOptions } from '@/features/collections/refs'
import { useCollectionMutations } from '@/features/collections/useCollectionMutations'
import { HomePane, type HomeView } from '@/features/home/HomePane'
import { useHomeSelection } from '@/features/home/useHomeSelection'
import { LibraryRail } from '@/features/library/LibraryRail'
import { useLibrary, type LibraryCatalog, type LibraryCollection } from '@/features/library/useLibrary'
import { useEditorGuard } from './EditorGuard'
import type { EditorTarget } from './target'

/** Which region a screen too narrow for both is showing. */
type MobileView = 'library' | 'pane'

/**
 * The builder's two regions and the state that spans them.
 *
 * The Library rail on the left is the source you pick from; the pane on the
 * right holds exactly one thing — your home screen, or one editor. Selecting a
 * rail row fills the pane with its editor; closing the editor gives the pane
 * back to home.
 *
 * **Below `lg` the two take turns**, because there is only one column and a
 * rail is taller than a screen — stacked, selecting a row opened an editor
 * below the fold and read as nothing happening at all. `mobileView` is held
 * here rather than in either region because the transitions belong to the same
 * four entry points the guard already covers: `show` moves to the pane,
 * `close` and a save go back to the rail, and `showHome` is the rail's own way
 * across. Nothing can move the pane without passing through them.
 *
 * **This owns every way out of an editor**, because every one of them starts
 * outside the editor: the × in its header, Escape, selecting a different row in
 * the rail, and switching profile in the header. An editor only reports whether
 * it has unsaved changes; the decision to warn is made here, once, so no exit
 * can be added later that quietly skips the check.
 */
export function Workspace({ profileIndex }: { profileIndex: number }) {
  const library = useLibrary(profileIndex)
  const home = useHomeSelection()

  const catalogMutations = useCatalogMutations(profileIndex)
  const collectionMutations = useCollectionMutations(profileIndex)

  const { guard, setDirty, blocked, proceed, cancel } = useEditorGuard()

  const [target, setTarget] = useState<EditorTarget | null>(null)
  // Which of the two regions the narrow layout is showing. Above `lg` both are
  // on screen at once and this is ignored.
  const [mobileView, setMobileView] = useState<MobileView>('library')
  // Held here, not in `HomePane`, so it survives an editor taking the pane:
  // closing one gives back the view you left rather than resetting to List.
  const [homeView, setHomeView] = useState<HomeView>('list')
  const [namingCatalog, setNamingCatalog] = useState(false)
  const [namingCollection, setNamingCollection] = useState(false)
  const [deletingCatalog, setDeletingCatalog] = useState<LibraryCatalog | null>(null)
  const [deletingCollection, setDeletingCollection] = useState<LibraryCollection | null>(null)

  const refOptions = useMemo(
    () => buildRefOptions(library.catalogs, library.genres),
    [library.catalogs, library.genres],
  )
  const refOptionByID = useMemo(() => indexRefOptions(refOptions), [refOptions])
  const refAccessible = useMemo(() => accessibleIDs(refOptions), [refOptions])

  const genres = useMemo(
    () => ({
      movie: [...library.genres.movie].map(([id, name]) => ({ id, name })),
      tv: [...library.genres.tv].map(([id, name]) => ({ id, name })),
    }),
    [library.genres],
  )

  // `reset` is stable per mutation, so hoisting the four makes `show` stable
  // too — which matters because it reaches `EditorShell`'s Escape listener.
  const resetCatalogCreate = catalogMutations.create.reset
  const resetCatalogUpdate = catalogMutations.update.reset
  const resetCollectionCreate = collectionMutations.create.reset
  const resetCollectionUpdate = collectionMutations.update.reset

  /**
   * Hand the pane to something else.
   *
   * Clears the dirty flag: the incoming editor reports its own, and a stale
   * `true` would guard a form that no longer exists. Clears the mutations'
   * errors for the same reason — a save that failed leaves its message behind,
   * and the next editor would open showing a rejection of something else.
   */
  const show = useCallback(
    (next: EditorTarget | null) => {
      setDirty(false)
      resetCatalogCreate()
      resetCatalogUpdate()
      resetCollectionCreate()
      resetCollectionUpdate()
      setTarget(next)
      // Opening something is also a request to look at it, which on a narrow
      // screen means the pane. Emptying the pane is not the reverse — where a
      // close lands is the caller's decision, so `null` leaves the view alone.
      if (next !== null) setMobileView('pane')
    },
    [setDirty, resetCatalogCreate, resetCatalogUpdate, resetCollectionCreate, resetCollectionUpdate],
  )

  const open = useCallback(
    (next: EditorTarget) => {
      // Re-selecting the row already open would re-seed the form from its
      // saved state, silently discarding edits. Nothing to do instead.
      if (
        next.sourceID !== undefined &&
        target?.sourceID === next.sourceID &&
        target.mode === next.mode
      ) {
        return
      }
      guard(() => show(next))
    },
    [guard, show, target],
  )

  /**
   * Close the editor.
   *
   * Narrow, this goes back to the rail rather than on to home: the rail is
   * where the editor was opened from, and the row is still selected there, so
   * its duplicate and delete actions are where they were left. Home is a place
   * you ask for, through the rail's own link to it.
   */
  const close = useCallback(
    () =>
      guard(() => {
        show(null)
        setMobileView('library')
      }),
    [guard, show],
  )

  /** Saved, so there is nothing left to warn about — straight back out. */
  function closeAfterSave() {
    show(null)
    setMobileView('library')
  }

  /** The rail's way into the pane when no editor is open. */
  function showHome() {
    guard(() => {
      show(null)
      setMobileView('pane')
    })
  }

  /**
   * "New catalog" creates the row, then hands it to the editor.
   *
   * The catalog exists before anything is filtered, so the name/type dialog and
   * the filter editor are two separate commitments rather than one long form.
   * That also means `type` — the one field that can't be changed afterwards —
   * is settled up front instead of buried among filters that can.
   *
   * The guard runs *after* the create, not before opening the dialog: an editor
   * with unsaved work shouldn't be discarded on the way to a decision the user
   * hasn't made yet. Declining here costs nothing — the catalog is in the rail
   * either way, ready to be selected later.
   */
  function createBareCatalog(name: string, type: CatalogType) {
    catalogMutations.create.mutate(toPayload({ ...emptyForm(type), name }), {
      onSuccess: (catalog) => {
        setNamingCatalog(false)
        // Anything you just made is yours, so this can't be a duplicate target.
        guard(() => show(catalogTarget({ ...catalog, owned: true })))
      },
    })
  }

  /** The collection half of the same two-step: title it, then fill it. */
  function createBareCollection(title: string) {
    collectionMutations.create.mutate(toCollectionPayload({ ...emptyCollectionForm(), title }), {
      onSuccess: (collection) => {
        setNamingCollection(false)
        guard(() =>
          show(
            collectionTarget(
              // `folders` is `Folder[] | null` on the wire and `Folder[]` in the
              // library; a collection created empty is exactly the case that
              // comes back null.
              { ...collection, owned: true, folders: collection.folders ?? [] },
              refAccessible,
            ),
          ),
        )
      },
    })
  }

  function saveCatalog(state: CatalogFormState) {
    const payload = toPayload(state)
    if (target?.kind === 'catalog' && target.mode === 'edit' && target.catalogID) {
      catalogMutations.update.mutate(
        { id: target.catalogID, payload },
        { onSuccess: closeAfterSave },
      )
    } else {
      catalogMutations.create.mutate(payload, { onSuccess: closeAfterSave })
    }
  }

  function saveCollection(state: CollectionFormState) {
    const payload = toCollectionPayload(state)
    if (target?.kind === 'collection' && target.mode === 'edit' && target.collectionID) {
      collectionMutations.update.mutate(
        { id: target.collectionID, payload },
        { onSuccess: closeAfterSave },
      )
    } else {
      collectionMutations.create.mutate(payload, { onSuccess: closeAfterSave })
    }
  }

  function confirmDeleteCatalog() {
    if (!deletingCatalog) return
    const id = deletingCatalog.id
    catalogMutations.remove.mutate(id, {
      onSuccess: () => {
        // A deleted catalog can't stay on the home screen — drop it from the
        // pending selection too, or Push would reject the stale reference.
        home.removeCatalog(id)
        setDeletingCatalog(null)
        // Nor can it stay in the pane. This is the one close that doesn't ask:
        // the row it was editing is gone, so there is nothing to go back to and
        // nothing left to save.
        if (target?.sourceID === id) show(null)
      },
    })
  }

  function confirmDeleteCollection() {
    if (!deletingCollection) return
    const id = deletingCollection.id
    collectionMutations.remove.mutate(id, {
      onSuccess: () => {
        home.removeCollection(id)
        setDeletingCollection(null)
        if (target?.sourceID === id) show(null)
      },
    })
  }

  // A 404 on a profile-scoped route means this slot was never selected —
  // there's nothing to retry, so send the user back to pick one.
  if (library.error instanceof ProfileNotSelectedError) {
    return <Navigate to="/profiles" replace />
  }

  const catalogSaving = catalogMutations.create.isPending || catalogMutations.update.isPending
  const collectionSaving =
    collectionMutations.create.isPending || collectionMutations.update.isPending

  return (
    <>
      <div className="grid min-h-0 flex-1 lg:grid-cols-[372px_minmax(0,1fr)]">
        <LibraryRail
          library={library}
          // One column below `lg`, so the two regions take turns instead of
          // stacking: a rail is hundreds of pixels tall, and an editor mounted
          // underneath one is an editor nobody sees open.
          className={mobileView === 'pane' ? 'hidden lg:flex' : 'flex'}
          homeSelected={target === null}
          onShowHome={showHome}
          selectedID={target?.sourceID ?? null}
          onNewCatalog={() => {
            // A rejection from a previous attempt — or from a duplicate, which
            // shares this mutation — must not greet the next one.
            resetCatalogCreate()
            setNamingCatalog(true)
          }}
          onNewCollection={() => {
            resetCollectionCreate()
            setNamingCollection(true)
          }}
          onSelectCatalog={(catalog) => open(catalogTarget(catalog))}
          onSelectCollection={(collection) => open(collectionTarget(collection, refAccessible))}
          onDuplicateCatalog={(catalog) =>
            open({
              kind: 'catalog',
              mode: 'duplicate',
              initial: formFromCatalog(catalog, 'duplicate'),
              sourceID: catalog.id,
            })
          }
          onDuplicateCollection={(collection) =>
            open(seedCollection(collection, 'duplicate', refAccessible))
          }
          onDeleteCatalog={setDeletingCatalog}
          onDeleteCollection={setDeletingCollection}
        />

        <div
          className={`min-w-0 flex-col lg:flex lg:min-h-0 ${
            mobileView === 'library' ? 'hidden' : 'flex'
          }`}
        >
          {target === null ? (
            <HomePane
              view={homeView}
              onViewChange={setHomeView}
              onBack={() => setMobileView('library')}
            />
          ) : target.kind === 'catalog' ? (
            <CatalogEditor
              // Remount on a different target rather than re-seeding in place:
              // the form, its validation and its preview are all per-catalog,
              // and a key is the honest way to say "this is a different
              // subject".
              key={target.sourceID ?? `new-${target.mode}`}
              mode={target.mode}
              initial={target.initial}
              genres={genres}
              certifications={library.certifications}
              countryNames={library.countryNames}
              languages={library.languages}
              saving={catalogSaving}
              serverError={
                (catalogMutations.create.error as Error | null)?.message ??
                (catalogMutations.update.error as Error | null)?.message ??
                null
              }
              onSave={saveCatalog}
              onRequestClose={close}
              onBack={() => setMobileView('library')}
              onDirtyChange={setDirty}
            />
          ) : (
            <CollectionEditor
              key={target.sourceID ?? `new-${target.mode}`}
              mode={target.mode}
              initial={target.initial}
              droppedRefs={target.droppedRefs}
              options={refOptions}
              optionByID={refOptionByID}
              accessibleIDs={refAccessible}
              saving={collectionSaving}
              serverError={
                (collectionMutations.create.error as Error | null)?.message ??
                (collectionMutations.update.error as Error | null)?.message ??
                null
              }
              onSave={saveCollection}
              onRequestClose={close}
              onBack={() => setMobileView('library')}
              onDirtyChange={setDirty}
            />
          )}
        </div>
      </div>

      <NewCatalogDialog
        open={namingCatalog}
        saving={catalogMutations.create.isPending}
        serverError={(catalogMutations.create.error as Error | null)?.message ?? null}
        onCreate={createBareCatalog}
        onClose={() => {
          setNamingCatalog(false)
          resetCatalogCreate()
        }}
      />

      <NewCollectionDialog
        open={namingCollection}
        saving={collectionMutations.create.isPending}
        serverError={(collectionMutations.create.error as Error | null)?.message ?? null}
        onCreate={createBareCollection}
        onClose={() => {
          setNamingCollection(false)
          resetCollectionCreate()
        }}
      />

      {/* Rendered here rather than beside the guard itself, because this is the
          level that knows what is being edited — the prompt names it. */}
      <ConfirmDialog
        open={blocked}
        title="Discard unsaved changes?"
        body={
          <>
            Your changes to <strong className="text-ink">{editorSubject(target)}</strong> haven't
            been saved. Leaving discards them.
          </>
        }
        confirmLabel="Discard"
        cancelLabel="Keep editing"
        destructive
        onConfirm={proceed}
        onCancel={cancel}
      />

      <ConfirmDialog
        open={deletingCatalog !== null}
        title="Delete this catalog?"
        body={
          <>
            <strong className="text-ink">{deletingCatalog?.name}</strong> is removed for{' '}
            <strong className="text-ink">everyone using it</strong>, not just you. This can't be
            undone.
          </>
        }
        confirmLabel={catalogMutations.remove.isPending ? 'Deleting…' : 'Delete catalog'}
        cancelLabel="Keep it"
        destructive
        onConfirm={confirmDeleteCatalog}
        onCancel={() => setDeletingCatalog(null)}
      />

      <ConfirmDialog
        open={deletingCollection !== null}
        title="Delete this collection?"
        body={
          <>
            <strong className="text-ink">{deletingCollection?.title}</strong> and its{' '}
            {folderCount(deletingCollection)} are removed for{' '}
            <strong className="text-ink">everyone using it</strong>, not just you. The catalogs
            inside it are kept. This can't be undone.
          </>
        }
        confirmLabel={collectionMutations.remove.isPending ? 'Deleting…' : 'Delete collection'}
        cancelLabel="Keep it"
        destructive
        onConfirm={confirmDeleteCollection}
        onCancel={() => setDeletingCollection(null)}
      />
    </>
  )
}

/**
 * Selecting a catalog row opens the only editor that row supports.
 *
 * Yours opens for editing. Someone else's opens as a duplicate — you can't
 * change a community catalog, and a read-only view of a form you can't submit
 * would be a dead end. Seeding the copy instead means the row is still
 * inspectable and the one thing you *can* do with it is already set up. A fresh
 * duplicate isn't dirty, so clicking through several community rows never
 * raises a prompt.
 */
function catalogTarget(catalog: LibraryCatalog): EditorTarget {
  const mode = catalog.owned ? 'edit' : 'duplicate'
  return {
    kind: 'catalog',
    mode,
    initial: formFromCatalog(catalog, mode),
    catalogID: catalog.owned ? catalog.id : undefined,
    sourceID: catalog.id,
  }
}

function collectionTarget(
  collection: LibraryCollection,
  accessible: ReadonlySet<string>,
): EditorTarget {
  return seedCollection(collection, collection.owned ? 'edit' : 'duplicate', accessible)
}

function seedCollection(
  collection: LibraryCollection,
  mode: 'edit' | 'duplicate',
  accessible: ReadonlySet<string>,
): EditorTarget {
  const { state, droppedRefs } = formFromCollection(collection, mode, accessible)
  return {
    kind: 'collection',
    mode,
    initial: state,
    droppedRefs,
    collectionID: mode === 'edit' ? collection.id : undefined,
    sourceID: collection.id,
  }
}

/** What the discard prompt is about. Named from the form's own title so it
 *  matches the header the user is looking at, including "Untitled". */
function editorSubject(target: EditorTarget | null): string {
  if (target === null) return 'this editor'
  if (target.kind === 'catalog') return target.initial.name.trim() || 'this catalog'
  return target.initial.title.trim() || 'this collection'
}

function folderCount(collection: LibraryCollection | null): string {
  const n = collection?.folders.length ?? 0
  return `${n} ${n === 1 ? 'folder' : 'folders'}`
}
