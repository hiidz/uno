import { useMemo, useState } from 'react'
import type { CSSProperties } from 'react'
import { TriangleAlert } from 'lucide-react'
import type {
  Catalog,
  CatalogType,
  CertificationsByCountry,
  Genre,
  Language,
} from '@/api'
import { CATALOG_PROVIDER } from '@/api'
import { moveByOne } from '@/components/dnd'
import { Field, Segmented, Switch, TextInput } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { Modal } from '@/components/Modal'
import { EditorFooter, SaveError } from '@/features/builder/EditorFooter'
import { EditorShell } from '@/features/builder/EditorShell'
import { NewItemDialog } from '@/features/builder/NewItemDialog'
import { useEditorForm } from '@/features/builder/useEditorForm'
import { CatalogEditor } from '@/features/catalogs/CatalogEditor'
import {
  emptyForm,
  formFromCatalog,
  toPayload as toCatalogPayload,
} from '@/features/catalogs/catalogForm'
import type { CatalogFormState } from '@/features/catalogs/catalogForm'
import type { CountryLookup } from '@/features/catalogs/countries'
import { useCatalogMutations } from '@/features/catalogs/useCatalogMutations'
import type { GenreLookups } from '@/features/library/useLibrary'
import { pluralCount } from '@/lib/plural'
import { CollectionPreview } from './CollectionPreview'
import { FolderCard, FolderTreeDnd } from './FolderCard'
import {
  VIEW_MODES,
  VIEW_MODE_LABELS,
  countErrors,
  emptyCollectionForm,
  isSameCollection,
  newFolder,
  removedFolders,
  validateCollectionForm,
  type CollectionFormState,
  type CollectionViewMode,
  type FolderFormState,
} from './collectionForm'
import { buildRefOptions, indexRefOptions, type RefOption } from './refs'

/**
 * Create or edit a collection, folders and catalog refs included, filling
 * the builder's right pane. Duplicating one is a separate, atomic server
 * call (`DuplicateCollection`) that hands back a finished
 * copy this editor then opens for editing like any other row — there is no
 * duplicate mode here any more.
 *
 * **The whole tree, one save.** `POST`/`PUT` replace the collection, its
 * folders and every folder's refs in a single transaction, so this editor has
 * one dirty state and one Save button rather than a save per folder.
 *
 * The consequence the UI has to state: **a folder dropped from the tree is
 * deleted server-side**, along with its refs. Nothing is committed until Save,
 * so it shows a standing note of what the next save would destroy rather than a
 * confirm on the remove button.
 *
 * **Dirtiness is reported, not handled.** Every way out originates outside this
 * component, so the pane owns the discard confirmation and this only says
 * whether there is anything to lose.
 *
 * **Three sources for a folder's catalogs**, per the closed-graph sharing
 * model: **link** one of your listed catalogs
 * (the original picker — a live pointer, edits to it reach every folder that
 * references it); **copy into this collection** (a fresh, scoped catalog only
 * this collection references, which nothing else can drift); **new inside
 * this collection** (the same, named first). The last two need this
 * collection to already have a server id, so they're offered only once it's
 * been saved at least once.
 *
 * **This editor keeps its own catalog registry** (`localCatalogs`), seeded
 * from `initialCatalogs` and grown by every scoped create/edit it makes —
 * `GET /api/p/{i}/catalogs` is listed catalogs only, so the library-wide
 * `options`/`optionByID`/`accessibleIDs` this editor is handed never contain
 * a scoped one. Folder rows render from this merged registry, never the
 * library alone, which is what makes a scoped catalog show at all.
 *
 * **Quiet Edit opens a catalog one level down**, in a `Modal` layered over
 * this editor rather than a second pane — the builder's pane holds one
 * occupant (`EditorShell`'s own doc comment), so a second, real editor has to
 * be a modal, not a stack. `CollectionEditor` stays mounted underneath, so
 * this editor's own unsaved folder edits survive the round trip.
 *
 * **There is no read-only "imported" view**, for the same reason the catalog
 * editor has none: the closed-graph sharing model has no such state — a
 * taken collection is a private copy, fully
 * yours from the moment it's created, folders and scoped catalogs included.
 * Every row this editor opens is yours.
 */
export function CollectionEditor({
  initial,
  options,
  optionByID,
  accessibleIDs,
  saving,
  serverError,
  onSave,
  onRequestClose,
  onDuplicate,
  onDelete,
  onDirtyChange,
  collectionID,
  initialCatalogs,
  profileIndex,
  genres,
  genreLookups,
  certifications,
  countryNames,
  languages,
  usedInPlaces,
}: {
  initial: CollectionFormState | null
  options: RefOption[]
  optionByID: ReadonlyMap<string, RefOption>
  accessibleIDs: ReadonlySet<string>
  saving: boolean
  /** Plain-text body of a server 400. Should be unreachable — the form mirrors
   *  every rule — so it renders as an unexpected-case banner, not a field. */
  serverError: string | null
  onSave: (state: CollectionFormState) => void
  /** Also backs the mobile Library button below `lg` — see `EditorShell`. */
  onRequestClose: () => void
  /** This collection's own row actions, carried in the header below `lg`.
   *  Absent while creating, when there is no row yet. */
  onDuplicate?: () => void
  onDelete?: () => void
  onDirtyChange: (dirty: boolean) => void
  /** This collection's own server id — present only once edit mode is seeded
   *  from a real row, or a brand-new one has been saved once. */
  collectionID?: string
  /** Every catalog this collection's folders already reference, listed or
   *  scoped — `[]` for a brand-new collection, which references nothing
   *  yet. A duplicated collection arrives here with its own fresh scoped
   *  copies already populated (`Workspace.tsx`'s `EditorTarget.initialCatalogs`
   *  override). Seeds `localCatalogs`. */
  initialCatalogs: Catalog[]
  /** For this editor's own instance of `useCatalogMutations` — deliberately
   *  its own, not `Workspace`'s: sharing one mutation object between the two
   *  would let a scoped copy/create here and the top-level "New catalog"
   *  dialog stomp each other's pending/error state. */
  profileIndex: number
  genres: { movie: Genre[]; tv: Genre[] }
  /** Same genres, shaped for `buildRefOptions` rather than the nested
   *  `CatalogEditor`'s own picker — building `RefOption`s for a freshly
   *  scoped catalog needs the lookup form, `Workspace`'s own already computed
   *  from this. */
  genreLookups: GenreLookups
  certifications: { movie: CertificationsByCountry; tv: CertificationsByCountry }
  countryNames: CountryLookup
  languages: Language[]
  /** Home screen plus every folder across every owned collection — computed
   *  in `Workspace`, which is the level that has the whole library and the
   *  home selection. Meaningful only for a listed catalog. */
  usedInPlaces: (catalogID: string) => number
}) {
  const catalogMutations = useCatalogMutations(profileIndex)

  const baseline = useMemo(() => initial ?? emptyCollectionForm(), [initial])
  const { state, setState, showErrors, submit } = useEditorForm(
    baseline,
    isSameCollection,
    onDirtyChange,
  )
  const dirty = !isSameCollection(baseline, state)

  // One folder open at a time, same shape as the catalog editor's collapsible
  // sections — DESIGN.md's "One folder is open at a time".
  const [openFolderKey, setOpenFolderKey] = useState<string | null>(null)

  // This editor's own catalog registry: the library (`optionByID`) plus every
  // scoped catalog it already knows about, grown by every copy/new/edit this
  // session makes. Never reseeded from `initialCatalogs` after mount — this
  // editor remounts on a different target (`key` in Workspace), so a fresh
  // instance always starts from the current seed.
  const [localCatalogs, setLocalCatalogs] = useState<Map<string, Catalog>>(
    () => new Map(initialCatalogs.map((c) => [c.id, c])),
  )
  function rememberCatalog(catalog: Catalog) {
    setLocalCatalogs((previous) => new Map(previous).set(catalog.id, catalog))
  }

  const localOptions = useMemo(
    () => buildRefOptions([...localCatalogs.values()], genreLookups),
    [localCatalogs, genreLookups],
  )
  const localOptionByID = useMemo(() => indexRefOptions(localOptions), [localOptions])
  const mergedOptionByID = useMemo(
    () => new Map([...optionByID, ...localOptionByID]),
    [optionByID, localOptionByID],
  )
  const mergedAccessibleIDs = useMemo(
    () => new Set([...accessibleIDs, ...localCatalogs.keys()]),
    [accessibleIDs, localCatalogs],
  )

  // A catalog this editor knows about, currently scoped to *this* collection.
  // These are exactly the rows `UpdateUserCollection`'s GC delete would drop
  // on Save if the last folder ref to one of them is gone — the save
  // bar's "N catalogs will be deleted" line below is this set filtered to
  // "not referenced by any folder in `state`".
  const scopedHere = useMemo(
    () =>
      collectionID === undefined
        ? []
        : [...localCatalogs.values()].filter((c) => c.collection_id === collectionID),
    [localCatalogs, collectionID],
  )
  const catalogsWillDelete = useMemo(
    () => scopedHere.filter((c) => !state.folders.some((f) => f.catalogIDs.includes(c.id))),
    [scopedHere, state.folders],
  )

  const errors = useMemo(
    () => validateCollectionForm(state, mergedAccessibleIDs),
    [state, mergedAccessibleIDs],
  )
  const errorCount = countErrors(errors)
  const willDelete = useMemo(() => removedFolders(baseline, state), [baseline, state])

  // The nested catalog editor — a modal layered over this pane, not a second
  // occupant of it. `null` means closed.
  const [nestedCatalogID, setNestedCatalogID] = useState<string | null>(null)
  // "New inside this collection" is named first, same two-step as the main
  // library's own "New catalog" — see Workspace's `createBareCatalog`.
  const [namingNewFolderKey, setNamingNewFolderKey] = useState<string | null>(null)
  const [newCatalogType, setNewCatalogType] = useState<CatalogType>('movie')

  function editRef(catalogID: string) {
    setNestedCatalogID(catalogID)
  }

  function closeNestedCatalog() {
    setNestedCatalogID(null)
  }

  function saveNestedCatalog(formState: CatalogFormState) {
    if (nestedCatalogID === null) return
    catalogMutations.update.mutate(
      { id: nestedCatalogID, payload: toCatalogPayload(formState) },
      {
        onSuccess: (catalog) => {
          rememberCatalog(catalog)
          setNestedCatalogID(null)
        },
      },
    )
  }

  /** "Copy" from the Add-catalogs picker: a fresh scoped catalog with the
   *  source's exact saved values (never re-derived through the form, so its
   *  `params` string round-trips byte for byte), referenced as a new ref. */
  function copyIntoCollection(folderKey: string, source: Catalog) {
    if (collectionID === undefined) return
    catalogMutations.create.mutate(
      {
        type: source.type,
        name: source.name,
        provider: CATALOG_PROVIDER,
        params: source.params,
        is_public: false,
        collection_id: collectionID,
      },
      { onSuccess: (copy) => { rememberCatalog(copy); addRef(folderKey, copy.id) } },
    )
  }

  /** "Copy into this collection" on an already-linked listed catalog's own
   *  row: same copy, but it replaces that ref in place rather than adding a
   *  second one, so the folder's order doesn't change. */
  function copyRefIntoCollection(folderKey: string, catalogID: string) {
    if (collectionID === undefined) return
    // A ref linked this session (via the picker) is a library catalog this
    // editor's own registry never had reason to remember — fall back to it.
    const source = localCatalogs.get(catalogID) ?? optionByID.get(catalogID)?.catalog
    if (!source) return
    catalogMutations.create.mutate(
      {
        type: source.type,
        name: source.name,
        provider: CATALOG_PROVIDER,
        params: source.params,
        is_public: false,
        collection_id: collectionID,
      },
      {
        onSuccess: (copy) => {
          rememberCatalog(copy)
          patchFolders((folders) =>
            folders.map((f) =>
              f.key === folderKey
                ? { ...f, catalogIDs: f.catalogIDs.map((id) => (id === catalogID ? copy.id : id)) }
                : f,
            ),
          )
        },
      },
    )
  }

  function startNewInCollection(folderKey: string) {
    catalogMutations.create.reset()
    setNewCatalogType('movie')
    setNamingNewFolderKey(folderKey)
  }

  function createNewInCollection(name: string) {
    if (namingNewFolderKey === null || collectionID === undefined) return
    const folderKey = namingNewFolderKey
    catalogMutations.create.mutate(
      toCatalogPayload({ ...emptyForm(newCatalogType, collectionID), name }),
      {
        onSuccess: (catalog) => {
          rememberCatalog(catalog)
          addRef(folderKey, catalog.id)
          setNamingNewFolderKey(null)
          setNestedCatalogID(catalog.id)
        },
      },
    )
  }

  function patch(update: Partial<CollectionFormState>) {
    setState((previous) => ({ ...previous, ...update }))
  }

  function patchFolders(update: (folders: FolderFormState[]) => FolderFormState[]) {
    setState((previous) => ({ ...previous, folders: update(previous.folders) }))
  }

  function patchFolder(key: string, update: Partial<FolderFormState>) {
    patchFolders((folders) => folders.map((f) => (f.key === key ? { ...f, ...update } : f)))
  }

  function reorderFolders(orderedKeys: string[]) {
    patchFolders((folders) =>
      orderedKeys
        .map((key) => folders.find((f) => f.key === key))
        .filter((f): f is FolderFormState => f !== undefined),
    )
  }

  function moveFolder(key: string, direction: -1 | 1) {
    patchFolders((folders) => {
      const ids = moveByOne(
        folders.map((f) => f.key),
        key,
        direction,
      )
      return ids.map((id) => folders.find((f) => f.key === id)!).filter(Boolean)
    })
  }

  function addRef(folderKey: string, catalogID: string) {
    patchFolders((folders) =>
      folders.map((f) =>
        // Guarded as well as filtered out of the picker: a repeat inside one
        // folder breaks `PRIMARY KEY (folder_id, catalog_id)` as a 500.
        f.key === folderKey && !f.catalogIDs.includes(catalogID)
          ? { ...f, catalogIDs: [...f.catalogIDs, catalogID] }
          : f,
      ),
    )
  }

  function removeRef(folderKey: string, catalogID: string) {
    patchFolders((folders) =>
      folders.map((f) =>
        f.key === folderKey
          ? { ...f, catalogIDs: f.catalogIDs.filter((id) => id !== catalogID) }
          : f,
      ),
    )
  }

  function moveRef(folderKey: string, catalogID: string, direction: -1 | 1) {
    patchFolders((folders) =>
      folders.map((f) =>
        f.key === folderKey ? { ...f, catalogIDs: moveByOne(f.catalogIDs, catalogID, direction) } : f,
      ),
    )
  }

  /** Re-inserts what the next Save would delete, at the end of the list —
   *  DESIGN.md's "Undo removing it". The folders still carry their original
   *  `key`, which is safe to reinsert: `willDelete` is exactly the baseline
   *  folders no longer present in `state.folders`, so no key collides. */
  function undoRemoving() {
    patchFolders((folders) => [...folders, ...willDelete])
  }

  function trySubmit() {
    if (errorCount > 0) {
      const badFolder = state.folders.find((f) => errors.folders[f.key])
      if (badFolder) setOpenFolderKey(badFolder.key)
    }
    submit(errorCount, onSave)
  }

  const roleLabels = showErrors
    ? (() => {
        const labels: string[] = []
        if (errors.title) labels.push('Title')
        state.folders.forEach((folder, index) => {
          const folderErrors = errors.folders[folder.key]
          if (folderErrors?.title) labels.push(`folder ${index + 1}’s title`)
          if (folderErrors?.catalogIDs) labels.push(`folder ${index + 1}’s catalogs`)
        })
        return labels
      })()
    : []

  const status =
    errorCount > 0 ? (
      <span className="ed-status is-error">
        <Icon icon={TriangleAlert} size={16} />
        {pluralCount(errorCount, 'thing')} {errorCount === 1 ? 'needs' : 'need'} fixing:{' '}
        {roleLabels.join(', ')}
      </span>
    ) : dirty ? (
      <span className="ed-status">
        Unsaved changes
        {willDelete.length > 0 && (
          <span className="text-dim"> · {pluralCount(willDelete.length, 'folder')} will be deleted</span>
        )}
        {catalogsWillDelete.length > 0 && (
          <span className="text-dim"> · {pluralCount(catalogsWillDelete.length, 'catalog')} will be deleted</span>
        )}
      </span>
    ) : (
      <span className="ed-status is-muted">No changes yet</span>
    )

  return (
    <EditorShell
      eyebrow="Edit collection"
      title={state.title.trim() || 'Untitled collection'}
      onRequestClose={onRequestClose}
      onDuplicate={onDuplicate}
      onDelete={onDelete}
      footer={
        <EditorFooter
          mode="edit"
          noun="collection"
          saving={saving}
          showErrors={showErrors}
          errorCount={errorCount}
          onCancel={onRequestClose}
          onSubmit={trySubmit}
          status={status}
          saveLabel="Save collection"
          cancelLabel="Discard changes"
        />
      }
    >
      <div className="ed-container">
        <div className="ed">
          <div className="ed-form">
            <div className="cr is-field">
              <label htmlFor="col-title" className="cr-role type-eyebrow">
                Title
              </label>
              <div className="cr-val">
                <TextInput
                  id="col-title"
                  value={state.title}
                  onChange={(title) => patch({ title })}
                  placeholder="Saturday night"
                  invalid={Boolean(showErrors && errors.title)}
                />
                {showErrors && errors.title && (
                  <p className="field-error">
                    <Icon icon={TriangleAlert} size={16} className="text-danger" />
                    <span>{errors.title}</span>
                  </p>
                )}
              </div>
            </div>

            <div className="cr is-switch">
              <span className="cr-role type-eyebrow">Sharing</span>
              <div className="cr-val">
                <Switch
                  checked={state.isPublic}
                  onChange={(isPublic) => patch({ isPublic })}
                  label={
                    state.isPublic
                      ? 'Shared, so anyone can import it — along with every catalog inside it'
                      : 'Not shared, only you can use it'
                  }
                />
              </div>
            </div>

            <div className="cr">
              <span className="cr-role type-eyebrow">Show first</span>
              <div className="cr-val ed-line">
                <Segmented
                  ariaLabel="Show first on the home screen"
                  value={state.pinToTop ? 'yes' : 'no'}
                  onChange={(value) => patch({ pinToTop: value === 'yes' })}
                  options={[
                    { value: 'no', label: 'No' },
                    { value: 'yes', label: 'Yes' },
                  ]}
                />
                <span className="ed-note">
                  {state.pinToTop
                    ? 'Pinned above every other row on your TV.'
                    : 'Sits after your catalog rows on your TV.'}
                </span>
              </div>
            </div>

            <div className="cr">
              <span className="cr-role type-eyebrow">How folders open</span>
              <div className="cr-val">
                <div className="choices" role="group" aria-label="How folders open">
                  {VIEW_MODES.map((viewMode) => (
                    <button
                      key={viewMode}
                      type="button"
                      className="choice"
                      aria-pressed={state.viewMode === viewMode}
                      onClick={() => patch({ viewMode: viewMode as CollectionViewMode })}
                    >
                      {VIEW_MODE_LABELS[viewMode]}
                    </button>
                  ))}
                </div>
              </div>
            </div>

            <div className="cr">
              <span className="cr-role type-eyebrow">“All” tab</span>
              <div className="cr-val ed-line">
                {/* Only Tabbed Grids has tabs to add one to. The value is left
                    alone while it's greyed — it's still what this collection
                    is set to, and applies again the moment the view mode goes
                    back to Tabbed Grids. */}
                <Segmented
                  ariaLabel='"All" tab'
                  value={state.showAllTab ? 'on' : 'off'}
                  onChange={(value) => patch({ showAllTab: value === 'on' })}
                  disabled={state.viewMode !== 'TABBED_GRID'}
                  options={[
                    { value: 'off', label: 'Off' },
                    { value: 'on', label: 'On' },
                  ]}
                />
                <span className="ed-note">
                  {state.viewMode === 'TABBED_GRID'
                    ? 'Adds a first tab holding every catalog in a folder at once.'
                    : 'Only does something with tabbed grids. It stays on for when you switch back.'}
                </span>
              </div>
            </div>

            <div className="cr">
              <span className="cr-role type-eyebrow">Background image</span>
              <div className="cr-val">
                <TextInput
                  value={state.backdropImageURL}
                  onChange={(backdropImageURL) => patch({ backdropImageURL })}
                  placeholder="https://…"
                />
              </div>
            </div>

            {willDelete.length > 0 && (
              <RemovalWarning names={willDelete.map((f) => f.title.trim() || 'an untitled folder')} onUndo={undoRemoving} />
            )}

            <div className="border-line mt-6 flex items-center gap-3 border-b pb-3">
              <span className="type-eyebrow flex-1">Folders</span>
              <span className="type-data text-dim text-[13px]">
                {pluralCount(state.folders.length, 'folder')}, in this order
              </span>
              <button
                type="button"
                className="btn-secondary btn-sm"
                onClick={() =>
                  patchFolders((folders) => {
                    const folder = newFolder()
                    setOpenFolderKey(folder.key)
                    return [...folders, folder]
                  })
                }
              >
                Add folder
              </button>
            </div>

            {state.folders.length === 0 ? (
              <p className="type-data text-dimmer m-0 py-3 text-[11px]">
                No folders yet. Add one, then put catalogs in it.
              </p>
            ) : (
              <FolderTreeDnd
                folderKeys={state.folders.map((f) => f.key)}
                onReorderFolders={reorderFolders}
                onReorderRefs={(folderKey, catalogIDs) => patchFolder(folderKey, { catalogIDs })}
              >
                <ul className="m-0 flex list-none flex-col p-0">
                  {state.folders.map((folder, index) => (
                    <FolderCard
                      key={folder.key}
                      folder={folder}
                      position={index}
                      total={state.folders.length}
                      open={openFolderKey === folder.key}
                      onToggleOpen={() =>
                        setOpenFolderKey((current) => (current === folder.key ? null : folder.key))
                      }
                      errors={showErrors ? errors.folders[folder.key] : undefined}
                      options={options}
                      optionByID={mergedOptionByID}
                      collectionID={collectionID}
                      usedInPlaces={usedInPlaces}
                      onChange={(update) => patchFolder(folder.key, update)}
                      onMove={(direction) => moveFolder(folder.key, direction)}
                      onRemove={() => {
                        patchFolders((folders) => folders.filter((f) => f.key !== folder.key))
                        setOpenFolderKey((current) => (current === folder.key ? null : current))
                      }}
                      onAddRef={(catalogID) => addRef(folder.key, catalogID)}
                      onCopyRefIntoCollection={(catalogID) => {
                        const source = localOptionByID.get(catalogID)?.catalog ?? optionByID.get(catalogID)?.catalog
                        if (source) copyIntoCollection(folder.key, source)
                      }}
                      onRemoveRef={(catalogID) => removeRef(folder.key, catalogID)}
                      onMoveRef={(catalogID, direction) => moveRef(folder.key, catalogID, direction)}
                      onEditRef={editRef}
                      onCopyRef={(catalogID) => copyRefIntoCollection(folder.key, catalogID)}
                      onAddNewInCollection={() => startNewInCollection(folder.key)}
                    />
                  ))}
                </ul>
              </FolderTreeDnd>
            )}
          </div>

          <CollectionPreview state={state} optionByID={mergedOptionByID} />
        </div>
      </div>

      <SaveError noun="collection" message={serverError} />

      {nestedCatalogID !== null &&
        (() => {
          // `localCatalogs` only ever holds scoped catalogs (seeded from
          // `initialCatalogs`, grown by copy/new-in-collection) — a listed
          // catalog opened via a plain link only ever lives in the library
          // map, never here, so Edit on one has to fall back to it.
          const catalog = localCatalogs.get(nestedCatalogID) ?? mergedOptionByID.get(nestedCatalogID)?.catalog
          if (!catalog) return null
          return (
            <Modal
              open
              onClose={closeNestedCatalog}
              labelledBy="nested-catalog-title"
              width="min(860px, 100%)"
            >
              {/* Resets the sticky offset `EditorShell` computes for the
                  outer app header — inside this modal there is no such
                  header to clear, and inheriting the real one would leave a
                  stray gap once the form scrolls on a narrow screen. `flex`
                  plus `overflow-hidden` gives `EditorShell`'s own
                  `lg:h-full` a bounded parent, the same shape the real app
                  shell gives it, so its internal header/footer stay put and
                  only the form between them scrolls. */}
              <div
                style={{ '--app-h': '0px' } as CSSProperties}
                className="flex max-h-[85vh] flex-col overflow-hidden"
              >
                <h2 id="nested-catalog-title" className="sr-only">
                  Edit {catalog.name}
                </h2>
                <CatalogEditor
                  key={catalog.id}
                  mode="edit"
                  initial={formFromCatalog(catalog, 'edit')}
                  genres={genres}
                  certifications={certifications}
                  countryNames={countryNames}
                  languages={languages}
                  saving={catalogMutations.update.isPending}
                  serverError={(catalogMutations.update.error as Error | null)?.message ?? null}
                  onSave={saveNestedCatalog}
                  onRequestClose={closeNestedCatalog}
                  onDirtyChange={() => {}}
                />
              </div>
            </Modal>
          )
        })()}

      <NewItemDialog
        open={namingNewFolderKey !== null}
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
        onCreate={createNewInCollection}
        onClose={() => {
          setNamingNewFolderKey(null)
          catalogMutations.create.reset()
        }}
      />
    </EditorShell>
  )
}

/**
 * DESIGN.md's "Standing removal warning": a saved folder dropped from the
 * tree is deleted server-side on the next Save, cascading its refs. Nothing
 * commits until then, so this states what would happen rather than confirming
 * something that already did.
 */
function RemovalWarning({
  names,
  onUndo,
}: {
  names: string[]
  onUndo: () => void
}) {
  const noun = names.length === 1 ? 'the folder' : 'the folders'
  const pronoun = names.length === 1 ? 'it' : 'them'

  return (
    <div className="border-line mt-6 grid grid-cols-[16px_minmax(0,1fr)] gap-x-2 border-y py-4">
      <Icon icon={TriangleAlert} size={16} className="text-danger mt-0.5" />
      <div className="flex flex-wrap items-baseline gap-x-1.5 gap-y-1.5">
        <span className="text-[14px] leading-[20px]">
          The next Save will delete {noun} {joinQuoted(names)}. Only you lose {pronoun} — a taker's
          own copy of this collection is unaffected. Nothing is gone yet.
        </span>
        <button
          type="button"
          onClick={onUndo}
          className="text-ink hover:text-dim text-[13px] font-medium transition-colors"
        >
          Undo removing {pronoun}
        </button>
        <span className="type-data text-dim text-[13px] normal-case">or Save to go ahead.</span>
      </div>
    </div>
  )
}

function joinQuoted(names: string[]): string {
  const quoted = names.map((name) => `“${name}”`)
  if (quoted.length <= 1) return quoted.join('')
  if (quoted.length === 2) return `${quoted[0]} and ${quoted[1]}`
  return `${quoted.slice(0, -1).join(', ')} and ${quoted[quoted.length - 1]}`
}
