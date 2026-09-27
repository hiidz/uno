import { useCallback, useMemo, useRef, useState } from 'react'
import type { ComponentProps } from 'react'
import { Navigate } from 'react-router-dom'
import { ProfileNotSelectedError } from '@/api'
import type { CatalogType, CollectionPayload, ImportResult } from '@/api'
import { ArrowDown } from 'lucide-react'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { Icon } from '@/components/Icon'
import { Toast } from '@/components/Toast'
import { useToast } from '@/components/useToast'
import { ExportDialog } from '@/features/bundle/ExportDialog'
import { ImportDialog } from '@/features/bundle/ImportDialog'
import { CatalogEditor } from '@/features/catalogs/CatalogEditor'
import {
  duplicatePayload,
  emptyForm,
  formFromCatalog,
  toPayload,
  type CatalogFormState,
} from '@/features/catalogs/catalogForm'
import { CatalogTypeField } from '@/features/catalogs/fields'
import { useCatalogMutations } from '@/features/catalogs/useCatalogMutations'
import { CollectionEditor } from '@/features/collections/CollectionEditor'
import {
  emptyCollectionForm,
  formFromCollection,
  toCollectionPayload,
} from '@/features/collections/collectionForm'
import { accessibleIDs, buildRefOptions, indexRefOptions } from '@/features/collections/refs'
import { useCollectionMutations } from '@/features/collections/useCollectionMutations'
import { HomePane, type HomeView } from '@/features/home/HomePane'
import { useHomeEdits } from '@/features/home/useHomeSelection'
import { LibrarySection } from '@/features/library/LibrarySection'
import { useLibrary, type LibraryCatalog, type LibraryCollection } from '@/features/library/useLibrary'
import { andList } from '@/lib/list'
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
  // Edits only: reading the selection here would re-render the whole
  // workspace, open editor included, on every change to the home screen.
  const home = useHomeEdits()

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
  const [transfer, setTransfer] = useState<'import' | 'export' | null>(null)
  const [toast, setToast] = useToast()

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

  // `reset` is stable per mutation, so hoisting these makes `show` stable too
  // — which matters because it reaches `EditorShell`'s Escape listener.
  const resetCatalogCreate = catalogMutations.create.reset
  const resetCatalogUpdate = catalogMutations.update.reset
  const resetCollectionCreate = collectionMutations.create.reset
  const resetCollectionUpdate = collectionMutations.update.reset

  /**
   * Hand the pane to something else, and say where that leaves the reader.
   *
   * Clears the dirty flag: the incoming editor reports its own, and a stale
   * `true` would guard a form that no longer exists. Clears the saves' errors
   * for the same reason — a save that failed leaves its message behind, and
   * the next editor would open showing a rejection of something else.
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
      resetCatalogUpdate()
      resetCollectionUpdate()
      setTarget(next)
      requestScroll(scrollTo)
    },
    [setDirty, resetCatalogUpdate, resetCollectionUpdate, requestScroll],
  )

  const open = useCallback(
    (next: EditorTarget) => {
      if (target?.kind === next.kind && target.id === next.id) {
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
        guard(() => show(catalogTarget(catalog)))
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
              { ...collection, folders: collection.folders ?? [] },
            ),
          ),
        )
      },
    })
  }

  function saveCatalog(state: CatalogFormState) {
    if (target?.kind !== 'catalog') return
    catalogMutations.update.mutate(
      { id: target.id, payload: toPayload(state) },
      { onSuccess: closeAfterSave },
    )
  }

  // `payload` already resolved every draft catalog into an inline `new`
  // spec inside `CollectionEditor` itself, which is the one place that has
  // `localCatalogs` to resolve them against — see its own `save`.
  function saveCollection(payload: CollectionPayload) {
    if (target?.kind !== 'collection') return
    collectionMutations.update.mutate({ id: target.id, payload }, { onSuccess: closeAfterSave })
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
        if (target?.id === id) show(null, 'rail')
      },
    })
  }

  function confirmDeleteCollection(collection: LibraryCollection) {
    const id = collection.id
    collectionMutations.remove.mutate(id, {
      onSuccess: () => {
        home.removeCollection(id)
        setConfirming(null)
        if (target?.id === id) show(null, 'rail')
      },
    })
  }

  // Every folder across every owned collection that references a listed
  // catalog — the folder half of the collection editor's "used in N places"
  // line, which adds the home screen itself. Lives here because this is the
  // level with the whole library; the collection editor only ever sees one
  // collection. Hoisted above the early return below: every Hook in this
  // component has to run on every render, guard or not.
  const usedInFolders = useCallback(
    (catalogID: string) => {
      let count = 0
      for (const collection of library.collections) {
        for (const folder of collection.folders) {
          if (folder.refs?.some((ref) => ref.catalog_id === catalogID)) count += 1
        }
      }
      return count
    },
    [library.collections],
  )

  // A 404 on a profile-scoped route means this slot was never selected —
  // there's nothing to retry, so send the user back to pick one.
  if (library.error instanceof ProfileNotSelectedError) {
    return <Navigate to="/profiles" replace />
  }

  // The library row the open editor was opened from. Below `lg` the editor's
  // own header carries that row's duplicate and delete — the row itself is a
  // screen-length scroll away — so it has to know which row it stands for. Its
  // `linked` drives the editor's linked banner, and is current after a save or
  // an Update because both refetch the library. Undefined until the library's
  // refetch lists a row that was just created or duplicated.
  const activeCatalog =
    target?.kind === 'catalog' ? library.catalogs.find((catalog) => catalog.id === target.id) : undefined
  const activeCollection =
    target?.kind === 'collection'
      ? library.collections.find((collection) => collection.id === target.id)
      : undefined

  // Every catalog the open collection's folders already reference, listed or
  // scoped — `CollectionEditor`'s seed for its own catalog registry.
  // `target.initialCatalogs` wins when the target set it directly (a
  // collection just duplicated, whose row may not have reached
  // `library.collections` yet — see `confirmDuplicateCollection`); otherwise
  // it falls back to a lookup by id, the ordinary case for a row selected
  // straight from the rail.
  const editingCollectionCatalogs =
    target?.kind === 'collection' ? (target.initialCatalogs ?? activeCollection?.catalogs ?? []) : []

  function selectCatalog(catalog: LibraryCatalog) {
    open(catalogTarget(catalog))
  }

  function selectCollection(collection: LibraryCollection) {
    open(collectionTarget(collection))
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
    catalogMutations.create.reset()
    setConfirming({ kind: 'duplicate-catalog', catalog })
  }

  function duplicateCollection(collection: LibraryCollection) {
    collectionMutations.duplicate.reset()
    setConfirming({ kind: 'duplicate-collection', collection })
  }

  // Duplicating a catalog is one server call — the same `create` mutation
  // `createBareCatalog` uses, seeded from an existing row (`duplicatePayload`)
  // instead of a bare name. The finished copy opens straight into its own
  // editor for review, as a duplicated collection does below.
  function confirmDuplicateCatalog(catalog: LibraryCatalog) {
    catalogMutations.create.mutate(duplicatePayload(catalog), {
      onSuccess: (newCatalog) => {
        setConfirming(null)
        open(catalogTarget(newCatalog))
      },
    })
  }

  // Duplicating a collection is one server call (`DuplicateCollection`,
  // which shares `TakeCollection`'s tree copy): a collection's scoped
  // catalogs can't be represented client-side without fetching them, so the
  // copy happens server-side. The finished copy opens straight into its own
  // editor for review, carrying its catalogs because the library may not
  // list it yet.
  function confirmDuplicateCollection(collection: LibraryCollection) {
    collectionMutations.duplicate.mutate(collection.id, {
      onSuccess: (newCollection) => {
        setConfirming(null)
        open({
          kind: 'collection',
          id: newCollection.id,
          initial: formFromCollection(newCollection),
          initialCatalogs: newCollection.catalogs ?? [],
        })
      },
    })
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
          // Under the closed-graph sharing model a taker holds an
          // independent copy (`taken_from` is nulled on this delete, per
          // ON DELETE SET NULL) — deleting a shared catalog never reaches
          // anyone else's copy, only your own row and this profile's own
          // folder refs to it.
          body: (
            <>
              <strong className="text-ink">{catalog.name}</strong> is deleted permanently.
              {catalog.is_public && (
                <>
                  {' '}
                  Anyone who already took a copy keeps theirs — this only removes it from the
                  community list.
                </>
              )}{' '}
              Any references to this catalog from a collection will also be removed. This can't
              be undone.
            </>
          ),
          confirmLabel: catalogMutations.remove.isPending ? 'Deleting…' : 'Delete catalog',
          cancelLabel: 'Keep it',
          destructive: true,
          pending: catalogMutations.remove.isPending,
          error: catalogMutations.remove.error?.message ?? null,
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
              Creates a copy of <strong className="text-ink">{catalog.name}</strong>.
            </>
          ),
          confirmLabel: catalogMutations.create.isPending ? 'Duplicating…' : 'Duplicate catalog',
          cancelLabel: 'Cancel',
          pending: catalogMutations.create.isPending,
          error: catalogMutations.create.error?.message ?? null,
          onConfirm: () => confirmDuplicateCatalog(catalog),
          onCancel: () => {
            catalogMutations.create.reset()
            setConfirming(null)
          },
        }
      }

      case 'delete-collection': {
        const { collection } = confirming
        const scoped = scopedCatalogCount(collection)
        return {
          title: 'Delete this collection?',
          // As with a catalog, a taker's own copy is untouched by this — but
          // unlike a catalog, "the catalogs inside it are kept" is only true
          // for listed ones: a scoped catalog has no life outside the
          // collection that scopes it and cascades with it.
          body: (
            <>
              <strong className="text-ink">{collection.title}</strong> and its{' '}
              {folderCount(collection)} are deleted permanently.
              {collection.is_public && (
                <>
                  {' '}
                  Anyone who already took a copy keeps theirs — this only removes it from the
                  community list.
                </>
              )}{' '}
              {scoped > 0
                ? `${pluralCount(scoped, 'catalog')} made only for this collection ${scoped === 1 ? 'goes' : 'go'} with it. Any other catalog referenced here is kept.`
                : 'The catalogs referenced here are kept.'}{' '}
              This can't be undone.
            </>
          ),
          confirmLabel: collectionMutations.remove.isPending ? 'Deleting…' : 'Delete collection',
          cancelLabel: 'Keep it',
          destructive: true,
          pending: collectionMutations.remove.isPending,
          error: collectionMutations.remove.error?.message ?? null,
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
          confirmLabel: collectionMutations.duplicate.isPending ? 'Duplicating…' : 'Duplicate collection',
          cancelLabel: 'Cancel',
          pending: collectionMutations.duplicate.isPending,
          error: collectionMutations.duplicate.error?.message ?? null,
          onConfirm: () => confirmDuplicateCollection(collection),
          onCancel: () => {
            collectionMutations.duplicate.reset()
            setConfirming(null)
          },
        }
      }
    }
  }

  const prompt = confirmation()

  return (
    <>
      <div className="grid min-h-0 flex-1 lg:grid-cols-[372px_minmax(0,1fr)]">
        {/* The sidebar: every catalog and collection you own, one permanently
            visible section with its own scroll region and its own name/genre
            filter. The closed-graph model means the library is exactly this —
            there's nothing else to browse here; taking someone else's public
            catalog or collection is the Community tab's job, not this rail's.

            Below `lg` the rail is the top of one long page rather than a
            column, with the pane stacked underneath it, and it ends in a
            shortcut *down* to home. */}
        <aside
          ref={railRef}
          tabIndex={-1}
          data-landing
          aria-label="Library"
          className="bg-sidebar border-line flex flex-col outline-none lg:min-h-0 lg:border-r"
        >
          <LibrarySection
            library={library}
            selectedID={target?.id ?? null}
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
            onImport={() => setTransfer('import')}
            onExport={() => setTransfer('export')}
            notice={<Toast toast={toast} />}
          />

          {/* Above `lg` home is simply the other half of the screen and needs
              no link. Below it the pane is further down the same page, so
              this is a shortcut down to it — hence `↓` and not `›`. Sticky
              as the rail's last child: it pins to the bottom of the viewport
              while the rail is on screen, and leaves with the rail's end as
              the pane scrolls in. It still guards, because arriving at home
              means the open editor is replaced by it. */}
          <button
            type="button"
            onClick={showHome}
            className="btn-secondary bg-raised-hi sticky bottom-4 z-10 mb-4 self-center lg:hidden"
          >
            Your home screen
            <Icon icon={ArrowDown} size={16} />
          </button>
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
              key={target.id}
              initial={target.initial}
              genres={library.genreLists}
              certifications={library.certifications}
              countryNames={library.countryNames}
              languages={library.languages}
              // Only `update`: `create` belongs to the naming dialog and to
              // Duplicate, which can run while this editor is open.
              saving={catalogMutations.update.isPending}
              serverError={catalogMutations.update.error?.message ?? null}
              onSave={saveCatalog}
              onRequestClose={close}
              onDuplicate={activeCatalog ? () => duplicateCatalog(activeCatalog) : undefined}
              onDelete={activeCatalog ? () => deleteCatalog(activeCatalog) : undefined}
              onDirtyChange={setDirty}
              linked={activeCatalog?.linked ?? false}
            />
          ) : (
            <CollectionEditor
              key={target.id}
              initial={target.initial}
              options={refOptions}
              optionByID={refOptionByID}
              accessibleIDs={refAccessible}
              // Only `update`, for the same reason as the catalog editor's.
              saving={collectionMutations.update.isPending}
              serverError={collectionMutations.update.error?.message ?? null}
              onSave={saveCollection}
              onRequestClose={close}
              onDuplicate={activeCollection ? () => duplicateCollection(activeCollection) : undefined}
              onDelete={activeCollection ? () => deleteCollection(activeCollection) : undefined}
              onDirtyChange={setDirty}
              collectionID={target.id}
              initialCatalogs={editingCollectionCatalogs}
              genres={library.genreLists}
              genreLookups={library.genres}
              certifications={library.certifications}
              countryNames={library.countryNames}
              languages={library.languages}
              usedInFolders={usedInFolders}
              linked={activeCollection?.linked ?? false}
            />
          )}
        </div>
      </div>

      {/* Two fields, because two of them are hard to change later: a name is
          how the row is found in the rail, and `type` is immutable once the
          catalog exists — every filter in the builder branches on it, and
          nothing changes it afterwards. */}
      <NewItemDialog
        open={namingCatalog}
        noun="catalog"
        label="Name"
        placeholder="Trending Sci-Fi"
        saving={catalogMutations.create.isPending}
        serverError={catalogMutations.create.error?.message ?? null}
        extra={<CatalogTypeField value={newCatalogType} onChange={setNewCatalogType} />}
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
        serverError={collectionMutations.create.error?.message ?? null}
        onCreate={createBareCollection}
        onClose={() => {
          setNamingCollection(false)
          resetCollectionCreate()
        }}
      />

      {/* Neither goes through the guard: both open over the pane and leave
          its occupant alone, and an import neither opens what it wrote nor
          adds it to home. */}
      <ExportDialog
        open={transfer === 'export'}
        profileIndex={profileIndex}
        catalogs={library.catalogs}
        collections={library.collections}
        preselected={target?.id ?? null}
        onClose={() => setTransfer(null)}
      />
      <ImportDialog
        open={transfer === 'import'}
        profileIndex={profileIndex}
        onClose={() => setTransfer(null)}
        onImported={(result) => {
          setTransfer(null)
          setToast({ text: importedText(result), tone: 'success' })
        }}
      />

      {/* Rendered here rather than beside the guard itself, because this is the
          level that knows what is being edited — the prompt names it. */}
      {prompt && <ConfirmDialog open {...prompt} />}
    </>
  )
}

/** Selecting a library row opens it for editing: everything in the library is
 *  yours. */
function catalogTarget(catalog: LibraryCatalog): EditorTarget {
  return { kind: 'catalog', id: catalog.id, initial: formFromCatalog(catalog) }
}

function collectionTarget(collection: LibraryCollection): EditorTarget {
  return { kind: 'collection', id: collection.id, initial: formFromCollection(collection) }
}

/** What the discard prompt is about: the row as it was opened, which is the
 *  name the rail still shows — a rename is itself one of the changes the
 *  prompt is asking about. */
function editorSubject(target: EditorTarget | null): string {
  if (target === null) return 'this editor'
  if (target.kind === 'catalog') return target.initial.name.trim() || 'this catalog'
  return target.initial.title.trim() || 'this collection'
}

/** Counts what the rail gained: the new listed catalogs and the new
 *  collections. A catalog imported inside a collection, or reused, is not a
 *  new rail row, so it isn't counted. */
function importedText(result: ImportResult): string {
  const parts: string[] = []
  if (result.catalogs.length > 0) parts.push(pluralCount(result.catalogs.length, 'catalog'))
  if (result.collections.length > 0) parts.push(pluralCount(result.collections.length, 'collection'))
  return parts.length > 0 ? `Imported ${andList(parts)}` : 'Imported nothing new'
}

function folderCount(collection: LibraryCollection): string {
  return pluralCount(collection.folders.length, 'folder')
}

/** How many of this collection's own catalogs (`Collection.catalogs`, every
 *  catalog its folders reference) are scoped to it specifically — the ones
 *  that cascade with the collection on delete. A listed catalog referenced
 *  here survives deletion. */
function scopedCatalogCount(collection: LibraryCollection): number {
  return (collection.catalogs ?? []).filter((c) => c.collection_id === collection.id).length
}
