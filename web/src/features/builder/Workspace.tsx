import { useCallback, useMemo, useRef, useState } from 'react'
import type { ComponentProps, RefObject } from 'react'
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
import type { OpenPublication } from '@/features/community/communityQuery'
import { HomePane, type HomeView } from '@/features/home/HomePane'
import { LibrarySection } from '@/features/library/LibrarySection'
import { useLibrary, type LibraryCatalog, type LibraryCollection } from '@/features/library/useLibrary'
import { usePushWaiting } from '@/features/push/usePushWaiting'
import { CatalogFromCommunity, CollectionFromCommunity } from '@/features/sharing/FromCommunityView'
import { useWorkspaceSharing } from '@/features/sharing/useWorkspaceSharing'
import { andList } from '@/lib/list'
import { pluralCount } from '@/lib/plural'
import { useEditorGuard } from './EditorGuard'
import { DeleteMessage } from './DeleteMessage'
import { catalogDeleteConsequences, collectionDeleteConsequences } from './deleteConsequences'
import { NewItemDialog } from './NewItemDialog'
import { EditorLayer } from './EditorLayer'
import { useScrollRequests, useStackedLayout, useStackedScroll } from './stacked'
import { homeMounted, selectionAction } from './selection'
import { catalogTarget, collectionTarget, type EditorTarget } from './target'


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
 * **Below `lg` the rail and the Home pane stack into one scrolling document**,
 * both always mounted, and an open editor covers that document as a layer
 * (`EditorLayer`) instead of joining it. Closing the layer leaves the reader
 * where they were. The two shortcuts between rail and Home are scroll requests
 * (`stacked.ts`); opening and closing an editor scroll nothing.
 *
 * **This owns every way out of an editor**, because every one of them starts
 * outside the editor: selecting a row in the rail, switching profile in the
 * header, the × in the editor's header, Escape, and below `lg` the browser's
 * Back. An editor only reports whether it has unsaved changes; the decision to
 * warn is made here, once, so no exit can be added later that quietly skips
 * the check.
 */
export function Workspace({
  profileIndex,
  onOpenPublication,
}: {
  profileIndex: number
  /** Leaves for Community with a publication's page open: a copy's Update…. */
  onOpenPublication: (publication: OpenPublication) => void
}) {
  const library = useLibrary(profileIndex)
  const catalogMutations = useCatalogMutations(profileIndex)
  const collectionMutations = useCollectionMutations(profileIndex)

  const { guard, setDirty, blocked, proceed, cancel, dirty } = useEditorGuard()

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
  const waitingForPush = usePushWaiting(profileIndex)
  const sharing = useWorkspaceSharing({
    profileIndex,
    genres: library.genres,
    dirty,
    waitingForPush,
    onToast: setToast,
  })

  // Leaving the Workspace tab unmounts the pane, so it passes the editor's
  // guard like every other way out, and clears its dirty flag as the tab
  // switch does.
  function openPublication(publication: OpenPublication) {
    guard(() => {
      setDirty(false)
      onOpenPublication(publication)
    })
  }

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
   * Hand the pane to something else, or empty it.
   *
   * Clears the dirty flag: the incoming editor reports its own, and a stale
   * `true` would guard a form that no longer exists. Clears the saves' errors
   * for the same reason — a save that failed leaves its message behind, and
   * the next editor would open showing a rejection of something else.
   */
  const show = useCallback(
    (next: EditorTarget | null) => {
      setDirty(false)
      resetCatalogUpdate()
      resetCollectionUpdate()
      setTarget(next)
    },
    [setDirty, resetCatalogUpdate, resetCollectionUpdate],
  )

  /** Selecting a library row (`selectionAction`): another row opens through
   *  the guard; the open one closes as × does. */
  const open = useCallback(
    (next: EditorTarget) => {
      const actions = {
        open: () => guard(() => show(next)),
        close: () => guard(() => show(null)),
      }
      actions[selectionAction(target, next)]()
    },
    [guard, show, target],
  )

  /** Close the editor, giving the pane back to home: ×, Escape, the footer's
   *  Close, and below `lg` the browser's Back. */
  const close = useCallback(() => guard(() => show(null)), [guard, show])

  /** Saved, so there is nothing left to warn about. */
  function closeAfterSave() {
    show(null)
  }

  /** The rail's shortcut down to Home, below `lg`. Not an exit: while an
   *  editor is open its layer covers the rail, so this moves the page and
   *  nothing else. */
  const showHome = useCallback(() => requestScroll('pane'), [requestScroll])

  /** Home's shortcut back up to the rail, below `lg`. Moves the page and
   *  nothing else. */
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

  function saveCatalog(id: string, state: CatalogFormState) {
    catalogMutations.update.mutate({ id, payload: toPayload(state) }, { onSuccess: closeAfterSave })
  }

  // `payload` already resolved every draft catalog into an inline `new`
  // spec inside `CollectionEditor` itself, which is the one place that has
  // `localCatalogs` to resolve them against — see its own `save`.
  function saveCollection(id: string, payload: CollectionPayload) {
    collectionMutations.update.mutate({ id, payload }, { onSuccess: closeAfterSave })
  }

  function confirmDeleteCatalog(catalog: LibraryCatalog) {
    const id = catalog.id
    catalogMutations.remove.mutate(id, {
      onSuccess: () => {
        // The Home pane drops the deleted row from its pending selection once
        // the lists refetch (`HomeSelectionProvider`); Nuvio keeps it until the
        // next push, which the list of changes says.
        setConfirming(null)
        // Nor can it stay in the pane. This is the one close that doesn't ask:
        // the row it was editing is gone, so there is nothing to go back to and
        // nothing left to save.
        if (target?.id === id) show(null)
      },
    })
  }

  function confirmDeleteCollection(collection: LibraryCollection) {
    const id = collection.id
    collectionMutations.remove.mutate(id, {
      onSuccess: () => {
        setConfirming(null)
        if (target?.id === id) show(null)
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
  // own header carries that row's duplicate and delete — the row itself is
  // under the editor's layer — so it has to know which row it stands for. Its
  // sharing state fills the editor's sharing setting, and decides whether the
  // pane is an editor or the view of a row from Community (`subscription`); it
  // is current after a save, a sharing call or an Update because each
  // refetches the library. Undefined until the library's refetch lists a row
  // that was just created or duplicated.
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
  // which shares a subscribe's tree copy): a collection's scoped
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
          body: (
            <DeleteMessage
              name={catalog.name}
              consequences={catalogDeleteConsequences(catalog, library.collections)}
            />
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
        return {
          title: 'Delete this collection?',
          body: (
            <DeleteMessage
              name={collection.title}
              consequences={collectionDeleteConsequences(collection)}
            />
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
            there's nothing else to browse here; adding someone else's published
            catalog or collection is the Community tab's job, not this rail's.

            Below `lg` the rail is the top of one long page rather than a
            column, with the Home pane stacked underneath it, and it ends in a
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
              the pane scrolls in. `z-20` ties the rail's sticky signs and wins on
              document order; the sticky header band (`z-30`) stays over it. */}
          <button
            type="button"
            onClick={showHome}
            className="btn-secondary bg-raised-hi sticky bottom-4 z-20 mb-4 self-center lg:hidden"
          >
            Your home screen
            <Icon icon={ArrowDown} size={16} />
          </button>
        </aside>

        <HomeSlot
          shown={homeMounted(target, stacked)}
          paneRef={paneRef}
          view={homeView}
          onViewChange={setHomeView}
          onShowLibrary={showLibraryFromHome}
        />

        {target === null ? null : (
          <EditorLayer
            stacked={stacked}
            label={editorSubject(target)}
            onRequestClose={close}
            fallbackFocus={railRef}
          >
            {activeCatalog?.subscription ? (
              <CatalogFromCommunity
                key={activeCatalog.id}
                catalog={activeCatalog}
                genres={library.genres}
                profileIndex={profileIndex}
                waitingForPush={waitingForPush.has(activeCatalog.id)}
                onClose={close}
                onDuplicate={() => duplicateCatalog(activeCatalog)}
                onDelete={() => deleteCatalog(activeCatalog)}
                onUpdate={() =>
                  openPublication({ id: activeCatalog.subscription!.publication_id, kind: 'catalog' })
                }
              />
            ) : activeCollection?.subscription ? (
              <CollectionFromCommunity
                key={activeCollection.id}
                collection={activeCollection}
                genres={library.genres}
                profileIndex={profileIndex}
                waitingForPush={waitingForPush.has(activeCollection.id)}
                onClose={close}
                onDuplicate={() => duplicateCollection(activeCollection)}
                onDelete={() => deleteCollection(activeCollection)}
                onUpdate={() =>
                  openPublication({ id: activeCollection.subscription!.publication_id, kind: 'collection' })
                }
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
                onSave={(state) => saveCatalog(target.id, state)}
                onRequestClose={close}
                onDuplicate={activeCatalog ? () => duplicateCatalog(activeCatalog) : undefined}
                onDelete={activeCatalog ? () => deleteCatalog(activeCatalog) : undefined}
                onDirtyChange={setDirty}
                {...sharing.catalogSharing(activeCatalog)}
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
                onSave={(payload) => saveCollection(target.id, payload)}
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
                {...sharing.collectionSharing(activeCollection)}
              />
            )}
          </EditorLayer>
        )}
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
        initialValue=""
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
          the way a catalog's `type` is. View mode, the All tab and the
          backdrop are all editable afterwards, and folders are the substance of
          the editor rather than something to guess at up front. */}
      <NewItemDialog
        open={namingCollection}
        noun="collection"
        label="Title"
        placeholder="Saturday night"
        initialValue=""
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
      {sharing.dialogs}
    </>
  )
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

interface HomeSlotProps {
  shown: boolean
  paneRef: RefObject<HTMLDivElement | null>
  view: HomeView
  onViewChange: (view: HomeView) => void
  onShowLibrary: () => void
}

/**
 * The Home pane's place in the grid: the pane column from `lg` while no editor
 * is open, and below `lg` the lower half of the page, under any editor's layer.
 *
 * `min-h` below `lg` is what makes the pane scrollable *to*: a short home
 * screen is shorter than the viewport, and the browser cannot scroll a document
 * past its own end — without a full screen of pane to travel into, asking for
 * its top lands somewhere short of it and reads as the scroll having failed.
 * Above `lg` the pane is a grid column of a fixed-height shell and `min-h-0`
 * restores that.
 */
function HomeSlot({ shown, paneRef, view, onViewChange, onShowLibrary }: HomeSlotProps) {
  if (!shown) return null
  return (
    <div
      ref={paneRef}
      className="flex min-h-[calc(100svh_-_var(--app-h))] min-w-0 scroll-mt-[var(--app-h)] flex-col lg:min-h-0"
    >
      <HomePane view={view} onViewChange={onViewChange} onShowLibrary={onShowLibrary} />
    </div>
  )
}
