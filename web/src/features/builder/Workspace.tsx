import { useCallback, useMemo, useRef, useState } from 'react'
import type { ComponentProps } from 'react'
import { Navigate } from 'react-router-dom'
import { ProfileNotSelectedError } from '@/api'
import type { CatalogType } from '@/api'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { Field, Segmented } from '@/components/fields'
import { CatalogEditor } from '@/features/catalogs/CatalogEditor'
import {
  emptyForm,
  formFromCatalog,
  toPayload,
  type CatalogFormState,
} from '@/features/catalogs/catalogForm'
import { useCatalogMutations } from '@/features/catalogs/useCatalogMutations'
import { CollectionEditor } from '@/features/collections/CollectionEditor'
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
import { LibrarySection } from '@/features/library/LibrarySection'
import { useLibrary, type LibraryCatalog, type LibraryCollection } from '@/features/library/useLibrary'
import { pluralCount } from '@/lib/plural'
import { useEditorGuard } from './EditorGuard'
import { NewItemDialog } from './NewItemDialog'
import {
  useScrollRequests,
  useStackedLayout,
  useStackedScroll,
  type ScrollDestination,
} from './stacked'
import type { EditorTarget } from './target'

/**
 * What the workspace is waiting on an answer to.
 *
 * The four row actions all ask before they act, and only one can be asked at a
 * time: the prompt is a modal over the whole builder, so there is nothing to
 * raise a second one with while one is up, and this can't express two.
 */
type Confirmation =
  | { kind: 'delete-catalog'; catalog: LibraryCatalog }
  | { kind: 'duplicate-catalog'; catalog: LibraryCatalog }
  | { kind: 'delete-collection'; collection: LibraryCollection }
  | { kind: 'duplicate-collection'; collection: LibraryCollection }

/** Everything the prompt needs except whether it is up, which is answered by
 *  there being one at all. */
type ConfirmProps = Omit<ComponentProps<typeof ConfirmDialog>, 'open'>

/**
 * The builder's two regions and the state that spans them.
 *
 * The Library rail on the left is the source you pick from; the pane on the
 * right holds exactly one thing — your home screen, or one editor. Selecting a
 * rail row fills the pane with its editor; closing the editor gives the pane
 * back to home.
 *
 * **Below `lg` the same two regions stack into one scrolling document** — rail
 * on top, pane underneath, both always mounted. Selecting a row scrolls the
 * page to the pane rather than replacing what's on screen with it. That is the
 * whole of the narrow layout: there is no second view, nothing is hidden, and
 * so there is no state here describing which of them is up.
 *
 * What replaces that state is a scroll request, because two of the regions'
 * destinations are not derivable from the target alone — emptying the pane
 * after a save goes back to the rail, while asking for home goes down to it.
 * Requests are made **inside** the guarded callback, never in the handler that
 * started it, which is what keeps a held confirmation from scrolling the page
 * out from under itself. See `stacked.ts`.
 *
 * **This owns every way out of an editor**, because every one of them starts
 * outside the editor: selecting a different row in the rail, the rail's link to
 * home, switching profile in the header, and — above `lg`, where they exist —
 * the × in the editor's header and Escape. An editor only reports whether it
 * has unsaved changes; the decision to warn is made here, once, so no exit can
 * be added later that quietly skips the check.
 *
 * Scrolling is not one of them. Below `lg` the Library button moves the
 * viewport and nothing else: the editor stays mounted and stays dirty, exactly
 * as it does above `lg` while the rail sits beside it.
 */
export function Workspace({ profileIndex }: { profileIndex: number }) {
  const library = useLibrary(profileIndex)
  const home = useHomeSelection()

  const catalogMutations = useCatalogMutations(profileIndex)
  const collectionMutations = useCollectionMutations(profileIndex)

  const { guard, setDirty, blocked, proceed, cancel } = useEditorGuard()

  const [target, setTarget] = useState<EditorTarget | null>(null)
  // Held here, not in `HomePane`, so it survives an editor taking the pane:
  // closing one gives back the view you left rather than resetting to List.
  const [homeView, setHomeView] = useState<HomeView>('list')
  const [namingCatalog, setNamingCatalog] = useState(false)
  // The naming dialog's second field, held here because it is the answer the
  // dialog is collecting rather than something it displays — `createBareCatalog`
  // is what reads it. Reset when the dialog is opened, not when it closes, so a
  // create the server rejected keeps the choice for the retry.
  const [newCatalogType, setNewCatalogType] = useState<CatalogType>('movie')
  const [namingCollection, setNamingCollection] = useState(false)
  const [confirming, setConfirming] = useState<Confirmation | null>(null)

  const railRef = useRef<HTMLElement>(null)
  const paneRef = useRef<HTMLDivElement>(null)
  const stacked = useStackedLayout()
  const { request, requestScroll } = useScrollRequests()
  useStackedScroll({ stacked, request, paneRef, railRef })

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
   * Hand the pane to something else, and say where that leaves the reader.
   *
   * Clears the dirty flag: the incoming editor reports its own, and a stale
   * `true` would guard a form that no longer exists. Clears the mutations'
   * errors for the same reason — a save that failed leaves its message behind,
   * and the next editor would open showing a rejection of something else.
   *
   * `scrollTo` is the caller's decision because emptying the pane means two
   * different things: a save is finished with the pane and belongs back at the
   * list, while asking for home is a request to look at what's now there.
   * Opening something is always a request to look at it, so that's the default.
   * Above `lg` both regions are already on screen and the request is ignored.
   */
  const show = useCallback(
    (next: EditorTarget | null, scrollTo: ScrollDestination = 'pane') => {
      setDirty(false)
      resetCatalogCreate()
      resetCatalogUpdate()
      resetCollectionCreate()
      resetCollectionUpdate()
      setTarget(next)
      requestScroll(scrollTo)
    },
    [
      setDirty,
      resetCatalogCreate,
      resetCatalogUpdate,
      resetCollectionCreate,
      resetCollectionUpdate,
      requestScroll,
    ],
  )

  const open = useCallback(
    (next: EditorTarget) => {
      if (
        next.sourceID !== undefined &&
        target?.sourceID === next.sourceID &&
        target.mode === next.mode
      ) {
        // Re-selecting the open row must not re-seed the form from its saved
        // state — that would silently discard edits. Stacked, though, tapping
        // the row you already have open is how you ask to be taken back to it,
        // so the one thing it still does is scroll.
        requestScroll('pane')
        return
      }
      guard(() => show(next))
    },
    [guard, show, target, requestScroll],
  )

  /**
   * Close the editor, giving the pane back to home.
   *
   * Above `lg` this is the × in the editor's header and Escape. Below it it's
   * also the editor header's Library button — leaving this row is an exit
   * either way, so it goes through the same guard rather than a scroll-only
   * shortcut that would silently drop unsaved edits. Lands on the rail rather
   * than the pane, because emptying the pane by leaving it is not a request to
   * go and look at what replaced it — the same reasoning `closeAfterSave`
   * below already uses.
   */
  const close = useCallback(() => guard(() => show(null, 'rail')), [guard, show])

  /** Saved, so there is nothing left to warn about — straight back to the list,
   *  which is where the next thing to work on is. */
  function closeAfterSave() {
    show(null, 'rail')
  }

  /** The rail's link to home: a scroll down to the pane, not a way across to
   *  it. Still guarded, because home replaces whatever editor is open. */
  function showHome() {
    guard(() => show(null, 'pane'))
  }

  /**
   * Home's own way back up to the list, below `lg`.
   *
   * **Not an exit**, so it doesn't guard: home has nothing of the editor's to
   * discard, and pushing the pane's content isn't unmounting anything. It moves
   * the viewport and nothing else. An open editor's own Library button is a
   * different case — reusing `close` above, not this — because leaving *that*
   * row means deselecting it.
   */
  const showLibraryFromHome = useCallback(() => requestScroll('rail'), [requestScroll])

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
  function createBareCatalog(name: string) {
    catalogMutations.create.mutate(toPayload({ ...emptyForm(newCatalogType), name }), {
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

  function confirmDeleteCatalog(catalog: LibraryCatalog) {
    const id = catalog.id
    catalogMutations.remove.mutate(id, {
      onSuccess: () => {
        // A deleted catalog can't stay on the home screen — drop it from the
        // pending selection too, or Push would reject the stale reference.
        home.removeCatalog(id)
        setConfirming(null)
        // Nor can it stay in the pane. This is the one close that doesn't ask:
        // the row it was editing is gone, so there is nothing to go back to and
        // nothing left to save. Back to the rail rather than the home screen
        // that takes the pane's place — deleting is finished with the pane, and
        // stacked, the alternative is leaving the reader parked at a region
        // that just changed under them into something they didn't ask for.
        if (target?.sourceID === id) show(null, 'rail')
      },
    })
  }

  function confirmDeleteCollection(collection: LibraryCollection) {
    const id = collection.id
    collectionMutations.remove.mutate(id, {
      onSuccess: () => {
        home.removeCollection(id)
        setConfirming(null)
        if (target?.sourceID === id) show(null, 'rail')
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

  // The library row the open editor was opened from, when that row is one of
  // yours. Below `lg` the editor's own header carries that row's duplicate and
  // delete — the row itself is a screen-length scroll away — so it has to know
  // which row it stands for. A brand-new catalog has no row yet, and a
  // community row has neither action, so both come back undefined.
  const activeCatalog =
    target?.kind === 'catalog' && target.sourceID !== undefined
      ? library.catalogs.find((catalog) => catalog.id === target.sourceID && catalog.owned)
      : undefined
  const activeCollection =
    target?.kind === 'collection' && target.sourceID !== undefined
      ? library.collections.find(
          (collection) => collection.id === target.sourceID && collection.owned,
        )
      : undefined

  // Named rather than inlined at each call site: both Library sections —
  // "Mine" and "Community" — select into the same pane, so this is one
  // function shared by two rows of JSX rather than two closures doing the
  // same thing.
  function selectCatalog(catalog: LibraryCatalog) {
    open(catalogTarget(catalog))
  }

  function selectCollection(collection: LibraryCollection) {
    open(collectionTarget(collection, refAccessible))
  }

  // Named rather than inlined at the call sites, because the rail's row and the
  // editor's header are two places asking for the same four things. Both routes
  // land on the icon-only action from `LibraryItem`, which reads as "delete" as
  // easily as "duplicate" without its label — so each raises the prompt rather
  // than acting on the tap.
  function deleteCatalog(catalog: LibraryCatalog) {
    catalogMutations.remove.reset()
    setConfirming({ kind: 'delete-catalog', catalog })
  }

  function deleteCollection(collection: LibraryCollection) {
    collectionMutations.remove.reset()
    setConfirming({ kind: 'delete-collection', collection })
  }

  function duplicateCatalog(catalog: LibraryCatalog) {
    setConfirming({ kind: 'duplicate-catalog', catalog })
  }

  function duplicateCollection(collection: LibraryCollection) {
    setConfirming({ kind: 'duplicate-collection', collection })
  }

  function confirmDuplicateCatalog(catalog: LibraryCatalog) {
    setConfirming(null)
    open({
      kind: 'catalog',
      mode: 'duplicate',
      initial: formFromCatalog(catalog, 'duplicate'),
      sourceID: catalog.id,
    })
  }

  function confirmDuplicateCollection(collection: LibraryCollection) {
    setConfirming(null)
    open(seedCollection(collection, 'duplicate', refAccessible))
  }

  /**
   * The prompt this level is raising, if any, and what it says.
   *
   * One dialog rather than five. The discard guard and the four row actions
   * differ only in their copy and in what answering yes does, so they are one
   * set of props built from one piece of state — a sixth question added later
   * inherits the same shape instead of arriving as a sixth element that has to
   * be kept in step with the other five.
   *
   * The guard is asked first when both could be: it stands for a request to
   * leave the editor, which is the thing already in progress.
   */
  function confirmation(): ConfirmProps | null {
    if (blocked) {
      return {
        title: 'Discard unsaved changes?',
        body: (
          <>
            Your changes to <strong className="text-ink">{editorSubject(target)}</strong> haven't
            been saved. Leaving discards them.
          </>
        ),
        confirmLabel: 'Discard',
        cancelLabel: 'Keep editing',
        destructive: true,
        onConfirm: proceed,
        onCancel: cancel,
      }
    }

    if (confirming === null) return null

    switch (confirming.kind) {
      case 'delete-catalog': {
        const { catalog } = confirming
        return {
          title: 'Delete this catalog?',
          // Delete only ever reaches an owned item (`LibrarySection` wires
          // `onDelete` for owned rows alone), so the axis that actually varies
          // here is `is_public`, not who owns it: a catalog shared with the
          // community can be sitting on someone else's home screen right now,
          // where a private one can only ever be on yours.
          body: catalog.is_public ? (
            <>
              <strong className="text-ink">{catalog.name}</strong> is removed for{' '}
              <strong className="text-ink">everyone using it</strong>, not just you — it's shared
              with the community. This can't be undone.
            </>
          ) : (
            <>
              <strong className="text-ink">{catalog.name}</strong> is deleted permanently.
              Any references to this catalog from a collection will also be removed.
              This can't be undone.
            </>
          ),
          confirmLabel: catalogMutations.remove.isPending ? 'Deleting…' : 'Delete catalog',
          cancelLabel: 'Keep it',
          destructive: true,
          pending: catalogMutations.remove.isPending,
          error: (catalogMutations.remove.error as Error | null)?.message ?? null,
          onConfirm: () => confirmDeleteCatalog(catalog),
          onCancel: () => {
            catalogMutations.remove.reset()
            setConfirming(null)
          },
        }
      }

      case 'duplicate-catalog': {
        const { catalog } = confirming
        return {
          title: 'Duplicate this catalog?',
          body: (
            <>
              Creates a new, editable copy of{' '}
              <strong className="text-ink">{catalog.name}</strong>. The original is left untouched.
            </>
          ),
          confirmLabel: 'Duplicate catalog',
          cancelLabel: 'Cancel',
          onConfirm: () => confirmDuplicateCatalog(catalog),
          onCancel: () => setConfirming(null),
        }
      }

      case 'delete-collection': {
        const { collection } = confirming
        return {
          title: 'Delete this collection?',
          body: collection.is_public ? (
            <>
              <strong className="text-ink">{collection.title}</strong> and its{' '}
              {folderCount(collection)} are removed for{' '}
              <strong className="text-ink">everyone using it</strong>, not just you — it's shared
              with the community. The catalogs inside it are kept. This can't be undone.
            </>
          ) : (
            <>
              <strong className="text-ink">{collection.title}</strong> and its{' '}
              {folderCount(collection)} are deleted permanently. The catalogs inside it are kept.
              This can't be undone.
            </>
          ),
          confirmLabel: collectionMutations.remove.isPending ? 'Deleting…' : 'Delete collection',
          cancelLabel: 'Keep it',
          destructive: true,
          pending: collectionMutations.remove.isPending,
          error: (collectionMutations.remove.error as Error | null)?.message ?? null,
          onConfirm: () => confirmDeleteCollection(collection),
          onCancel: () => {
            collectionMutations.remove.reset()
            setConfirming(null)
          },
        }
      }

      case 'duplicate-collection': {
        const { collection } = confirming
        return {
          title: 'Duplicate this collection?',
          body: (
            <>
              Creates a new, editable copy of{' '}
              <strong className="text-ink">{collection.title}</strong>. The original is left
              untouched.
            </>
          ),
          confirmLabel: 'Duplicate collection',
          cancelLabel: 'Cancel',
          onConfirm: () => confirmDuplicateCollection(collection),
          onCancel: () => setConfirming(null),
        }
      }
    }
  }

  const prompt = confirmation()

  return (
    <>
      <div className="grid min-h-0 flex-1 lg:grid-cols-[372px_minmax(0,1fr)]">
        {/* The sidebar: your own catalogs and collections on top, everyone
            else's below. "Mine" and "community" are different tasks (build
            here, browse there), so each gets its own permanently visible
            section with its own scroll region and its own name/genre filter,
            rather than one filtered list with a control choosing which owner
            to look at. Only "Mine" gets the New buttons; you can't create a
            community row, only adopt one by opening it.

            Below `lg` the rail is the top of one long page rather than a
            column, with the pane stacked underneath it. It keeps its link to
            home, which stops being a way *across* to the pane and becomes a
            shortcut *down* to it. */}
        <aside
          ref={railRef}
          tabIndex={-1}
          data-landing
          aria-label="Library"
          className="bg-sidebar border-line flex scroll-mt-[var(--app-h)] flex-col outline-none lg:min-h-0 lg:border-r"
        >
          {/* Above `lg` home is simply the other half of the screen and needs
              no link. Below it the pane is further down the same page, so
              this is a shortcut to it rather than a way across — hence `↓`
              and not `›`. It still guards, because arriving at home means the
              open editor is replaced by it. */}
          <button
            type="button"
            onClick={showHome}
            aria-current={target === null ? 'true' : undefined}
            className={`border-line hover:bg-raised flex items-center gap-3 border-b px-4 py-3 text-left transition-colors lg:hidden ${
              target === null ? 'bg-raised-hi' : ''
            }`}
          >
            <span className="type-display flex-1 text-[12px]">Your home screen</span>
            <span aria-hidden="true" className="type-data text-dimmer text-[13px] leading-none">
              ↓
            </span>
          </button>

          <LibrarySection
            owned
            library={library}
            selectedID={target?.sourceID ?? null}
            onNewCatalog={() => {
              // A rejection from a previous attempt — or from a duplicate,
              // which shares this mutation — must not greet the next one.
              resetCatalogCreate()
              setNewCatalogType('movie')
              setNamingCatalog(true)
            }}
            onNewCollection={() => {
              resetCollectionCreate()
              setNamingCollection(true)
            }}
            onSelectCatalog={selectCatalog}
            onSelectCollection={selectCollection}
            onDuplicateCatalog={duplicateCatalog}
            onDuplicateCollection={duplicateCollection}
            onDeleteCatalog={deleteCatalog}
            onDeleteCollection={deleteCollection}
          />
          <LibrarySection
            owned={false}
            library={library}
            selectedID={target?.sourceID ?? null}
            onSelectCatalog={selectCatalog}
            onSelectCollection={selectCollection}
            onDuplicateCatalog={duplicateCatalog}
            onDuplicateCollection={duplicateCollection}
            onDeleteCatalog={deleteCatalog}
            onDeleteCollection={deleteCollection}
          />
        </aside>

        {/* `min-h` below `lg` is what makes the pane scrollable *to*: a short
            form is shorter than the viewport, and the browser cannot scroll a
            document past its own end — without a full screen of pane to travel
            into, asking for its top lands somewhere short of it and reads as
            the scroll having failed. Above `lg` the pane is a grid column of a
            fixed-height shell and `min-h-0` restores that. */}
        <div
          ref={paneRef}
          className="flex min-h-[calc(100svh_-_var(--app-h))] min-w-0 scroll-mt-[var(--app-h)] flex-col lg:min-h-0"
        >
          {target === null ? (
            <HomePane
              view={homeView}
              onViewChange={setHomeView}
              onShowLibrary={showLibraryFromHome}
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
              onDuplicate={activeCatalog ? () => duplicateCatalog(activeCatalog) : undefined}
              onDelete={activeCatalog ? () => deleteCatalog(activeCatalog) : undefined}
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
              onDuplicate={activeCollection ? () => duplicateCollection(activeCollection) : undefined}
              onDelete={activeCollection ? () => deleteCollection(activeCollection) : undefined}
              onDirtyChange={setDirty}
            />
          )}
        </div>
      </div>

      {/* Two fields, because two of them are hard to change later: a name is
          how the row is found in the rail, and `type` is immutable once the
          catalog exists — every filter in the builder branches on it, and the
          supported way to change one is to duplicate. */}
      <NewItemDialog
        open={namingCatalog}
        noun="catalog"
        label="Name"
        placeholder="Trending Sci-Fi"
        saving={catalogMutations.create.isPending}
        serverError={(catalogMutations.create.error as Error | null)?.message ?? null}
        extra={
          <Field label="Type" hint="Can't be changed later.">
            <Segmented
              ariaLabel="Catalog type"
              value={newCatalogType}
              onChange={setNewCatalogType}
              options={[
                { value: 'movie', label: 'Movie' },
                { value: 'series', label: 'Series' },
              ]}
            />
          </Field>
        }
        onCreate={createBareCatalog}
        onClose={() => {
          setNamingCatalog(false)
          resetCatalogCreate()
        }}
      />

      {/* One field, by the same rule: nothing about a collection is immutable
          the way a catalog's `type` is. View mode, pinning, the All tab and the
          backdrop are all editable afterwards, and folders are the substance of
          the editor rather than something to guess at up front. */}
      <NewItemDialog
        open={namingCollection}
        noun="collection"
        label="Title"
        placeholder="Saturday night"
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
      {prompt && <ConfirmDialog open {...prompt} />}
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

function folderCount(collection: LibraryCollection): string {
  return pluralCount(collection.folders.length, 'folder')
}
