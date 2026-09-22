import { useMemo, useState } from 'react'
import type { CSSProperties } from 'react'
import { TriangleAlert } from 'lucide-react'
import type {
  Catalog,
  CatalogType,
  CertificationsByCountry,
  CollectionPayload,
  Genre,
  Language,
} from '@/api'
import { CATALOG_PROVIDER } from '@/api'
import { ConfirmDialog } from '@/components/ConfirmDialog'
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
import { FolderDetail, FolderTiles, FolderTreeDnd, folderLabel } from './FolderCard'
import {
  DRAFT_ID_PREFIX,
  VIEW_MODES,
  VIEW_MODE_LABELS,
  countErrors,
  emptyCollectionForm,
  isDraftCatalogID,
  hasRef,
  isSameCollection,
  newFolder,
  newRef,
  removedFolders,
  reorderRefs,
  toCollectionPayload,
  validateCollectionForm,
  type CollectionFormState,
  type CollectionViewMode,
  type FolderFormState,
  type FolderRefState,
} from './collectionForm'
import { buildRefOptions, indexRefOptions, type RefOption } from './refs'

/** A catalog staged locally by "copy into this collection"/"new inside this
 *  collection" — not written to the DB until this collection's own Save,
 *  which resolves it into an inline `new` spec (`toCollectionPayload`). Kept
 *  in the same `localCatalogs` registry as every other catalog this editor
 *  knows about, so nothing downstream of that registry needs to tell a
 *  draft apart from a real row to render it. */
function draftCatalog(seed: {
  type: CatalogType
  name: string
  params: string
  collectionID: string
}): Catalog {
  return {
    id: `${DRAFT_ID_PREFIX}${crypto.randomUUID()}`,
    type: seed.type,
    name: seed.name,
    provider: CATALOG_PROVIDER,
    params: seed.params,
    owner_id: '',
    is_public: false,
    collection_id: seed.collectionID,
    created_at: '',
    updated_at: '',
  }
}

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
  /** Receives the finished payload, not the raw form state — resolving a
   *  draft catalog into its inline `new` spec needs this editor's own
   *  `localCatalogs`, which the caller (`Workspace.tsx`) doesn't have. */
  onSave: (payload: CollectionPayload) => void
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

  // The folder tile whose contents show under the strip — DESIGN.md's
  // "Folder strip". `null` until one is picked; see `selectedIndex`.
  const [selectedFolderKey, setSelectedFolderKey] = useState<string | null>(null)

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

  // A saved catalog this editor knows about, currently scoped to *this*
  // collection. These are exactly the rows `UpdateUserCollection`'s GC delete
  // would drop on Save if the last folder ref to one of them is gone — the save
  // bar's "N catalogs will be deleted" line below is this set filtered to
  // "not referenced by any folder in `state`". A draft is not a row, and an
  // unreferenced one is never sent, so drafts are left out.
  const scopedHere = useMemo(
    () =>
      collectionID === undefined
        ? []
        : [...localCatalogs.values()].filter((c) => c.collection_id === collectionID && !isDraftCatalogID(c.id)),
    [localCatalogs, collectionID],
  )
  const catalogsWillDelete = useMemo(
    () => scopedHere.filter((c) => !state.folders.some((f) => f.refs.some((ref) => ref.catalogID === c.id))),
    [scopedHere, state.folders],
  )

  // Legal to save — the server accepts a folder with no catalog refs — but it
  // shows nothing on the real TV, so it's surfaced as a standing warning
  // alongside the deletion counts rather than left to the folder's own,
  // possibly-never-opened panel note.
  const emptyFolders = useMemo(() => state.folders.filter((f) => f.refs.length === 0), [state.folders])

  // Listed catalogs, kept private on purpose (`collection_id === null`, per
  // `refs.ts`'s `accessibleIDs`), that a folder here references. A scoped
  // catalog has no separate private life to expose — it only exists inside
  // this collection — so it's excluded; this is only about a private catalog
  // that lives elsewhere in the library too, whose recipe a public collection
  // would hand to anyone who takes it.
  const privateReferencedCatalogs = useMemo(() => {
    const referencedIDs = new Set(state.folders.flatMap((f) => f.refs.map((ref) => ref.catalogID)))
    const seen = new Map<string, Catalog>()
    for (const id of referencedIDs) {
      const catalog = mergedOptionByID.get(id)?.catalog
      if (catalog && !catalog.is_public && catalog.collection_id === null) seen.set(id, catalog)
    }
    return [...seen.values()]
  }, [state.folders, mergedOptionByID])

  const errors = useMemo(
    () => validateCollectionForm(state, mergedAccessibleIDs),
    [state, mergedAccessibleIDs],
  )
  const errorCount = countErrors(errors)
  const willDelete = useMemo(() => removedFolders(baseline, state), [baseline, state])

  // The nested catalog editor — a modal layered over this pane, not a second
  // occupant of it. `null` means closed.
  const [nestedCatalogID, setNestedCatalogID] = useState<string | null>(null)
  // The nested form's unsaved edits aren't this pane's, so they don't reach
  // the pane's `EditorGuard`; closing the modal asks about them here instead.
  const [nestedDirty, setNestedDirty] = useState(false)
  const [confirmingNestedDiscard, setConfirmingNestedDiscard] = useState(false)
  const nestedCatalog = nestedCatalogID === null ? undefined : localCatalogs.get(nestedCatalogID)
  // Keyed on the catalog object, which only changes when `rememberCatalog`
  // replaces it: a fresh form on every render of this editor would re-seed the
  // nested one (see `useEditorForm`) and drop its edits when a save fails.
  const nestedInitial = useMemo(() => nestedCatalog && formFromCatalog(nestedCatalog), [nestedCatalog])
  // "New inside this collection" is named first, same two-step as the main
  // library's own "New catalog" — see Workspace's `createBareCatalog`.
  const [namingNewFolderKey, setNamingNewFolderKey] = useState<string | null>(null)
  const [newCatalogType, setNewCatalogType] = useState<CatalogType>('movie')

  function editRef(catalogID: string) {
    setNestedCatalogID(catalogID)
  }

  /** Every way out of the nested modal — ×, Cancel, Escape, the scrim. */
  function closeNestedCatalog() {
    if (nestedDirty) {
      setConfirmingNestedDiscard(true)
      return
    }
    setNestedCatalogID(null)
  }

  function discardNestedCatalog() {
    setConfirmingNestedDiscard(false)
    setNestedCatalogID(null)
  }

  /** Saves the nested editor's own row: a `PUT` for an already-real catalog
   *  (only ever a scoped one — a listed ref has no quiet Edit here, see
   *  `FolderCard.tsx`), or, for a draft, just a local replacement — nothing
   *  is written until this collection's own Save resolves it into an inline
   *  `new` spec (`toCollectionPayload`). */
  function saveNestedCatalog(formState: CatalogFormState) {
    if (nestedCatalogID === null) return
    const payload = toCatalogPayload(formState)
    if (isDraftCatalogID(nestedCatalogID)) {
      const draft = localCatalogs.get(nestedCatalogID)
      if (!draft) return
      rememberCatalog({ ...draft, type: payload.type, name: payload.name, params: payload.params })
      setNestedCatalogID(null)
      return
    }
    catalogMutations.update.mutate(
      { id: nestedCatalogID, payload },
      {
        onSuccess: (catalog) => {
          rememberCatalog(catalog)
          setNestedCatalogID(null)
        },
      },
    )
  }

  /** "Copy" from the Add-catalogs picker: a fresh scoped catalog staged
   *  locally with the source's exact saved values (never re-derived through
   *  the form, so its `params` string round-trips byte for byte), referenced
   *  as a new ref. Not written to the DB until this collection's own Save —
   *  see `draftCatalog`'s doc comment. */
  function copyIntoCollection(folderKey: string, source: Catalog) {
    if (collectionID === undefined) return
    const draft = draftCatalog({
      type: source.type,
      name: source.name,
      params: source.params,
      collectionID,
    })
    rememberCatalog(draft)
    addRef(folderKey, draft.id)
  }

  /** "Copy into this collection" on an already-linked listed catalog's own
   *  row: same staged copy, but it replaces that ref in place rather than
   *  adding a second one, so the folder's order doesn't change. */
  function copyRefIntoCollection(folderKey: string, refKey: string, catalogID: string) {
    if (collectionID === undefined) return
    // A ref linked this session (via the picker) is a library catalog this
    // editor's own registry never had reason to remember — fall back to it.
    const source = localCatalogs.get(catalogID) ?? optionByID.get(catalogID)?.catalog
    if (!source) return
    const draft = draftCatalog({
      type: source.type,
      name: source.name,
      params: source.params,
      collectionID,
    })
    rememberCatalog(draft)
    // Only this ref moves to the copy, keeping its genre; another ref to the
    // same listed catalog under a different genre stays linked.
    patchRefs(folderKey, (refs) => refs.map((ref) => (ref.key === refKey ? { ...ref, catalogID: draft.id } : ref)))
  }

  function startNewInCollection(folderKey: string) {
    setNewCatalogType('movie')
    setNamingNewFolderKey(folderKey)
  }

  /** "New inside this collection": named first (the naming dialog), then
   *  staged as an empty draft and opened in the nested editor to fill in
   *  filters — same two-step the library's own "New catalog" uses, but
   *  nothing is written to the DB until this collection's own Save. */
  function createNewInCollection(name: string) {
    if (namingNewFolderKey === null || collectionID === undefined) return
    const folderKey = namingNewFolderKey
    const form = { ...emptyForm(newCatalogType, collectionID), name }
    const draft = draftCatalog({
      type: form.type,
      name: toCatalogPayload(form).name,
      params: toCatalogPayload(form).params,
      collectionID,
    })
    rememberCatalog(draft)
    addRef(folderKey, draft.id)
    setNamingNewFolderKey(null)
    setNestedCatalogID(draft.id)
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

  /** Adds an unfiltered ref to `catalogID`. Guarded as well as filtered out of
   *  the picker: a second unfiltered ref to one catalog repeats the
   *  (catalog, genre) pair `folder_catalogs`' primary key forbids. */
  function addRef(folderKey: string, catalogID: string) {
    patchFolders((folders) =>
      folders.map((f) =>
        f.key === folderKey && !hasRef(f, catalogID, '') ? { ...f, refs: [...f.refs, newRef(catalogID)] } : f,
      ),
    )
  }

  /** "Add another genre" on a ref's own row: a second ref to the same catalog,
   *  under `genre`, directly below it. */
  function addGenreRef(folderKey: string, refKey: string, genre: string) {
    patchFolders((folders) =>
      folders.map((f) => {
        const at = f.refs.findIndex((ref) => ref.key === refKey)
        if (f.key !== folderKey || at === -1 || hasRef(f, f.refs[at].catalogID, genre)) return f
        const refs = [...f.refs]
        refs.splice(at + 1, 0, newRef(f.refs[at].catalogID, genre))
        return { ...f, refs }
      }),
    )
  }

  function patchRefs(folderKey: string, update: (refs: FolderRefState[]) => FolderRefState[]) {
    patchFolders((folders) => folders.map((f) => (f.key === folderKey ? { ...f, refs: update(f.refs) } : f)))
  }

  function removeRef(folderKey: string, refKey: string) {
    patchRefs(folderKey, (refs) => refs.filter((ref) => ref.key !== refKey))
  }

  function setRefGenre(folderKey: string, refKey: string, genre: string) {
    patchRefs(folderKey, (refs) => refs.map((ref) => (ref.key === refKey ? { ...ref, genre } : ref)))
  }

  function moveRef(folderKey: string, refKey: string, direction: -1 | 1) {
    patchRefs(folderKey, (refs) => reorderRefs(refs, moveByOne(refs.map((ref) => ref.key), refKey, direction)))
  }

  /** Re-inserts what the next Save would delete, at the end of the list —
   *  the removal warning's "Undo". The folders still carry their original
   *  `key`, which is safe to reinsert: `willDelete` is exactly the baseline
   *  folders no longer present in `state.folders`, so no key collides. */
  function undoRemoving() {
    patchFolders((folders) => [...folders, ...willDelete])
  }

  // One folder's contents always show under the strip: the one picked, or the
  // first when none has been (or the picked one was removed).
  const selectedIndex = Math.max(
    0,
    state.folders.findIndex((f) => f.key === selectedFolderKey),
  )
  const selectedFolder = state.folders[selectedIndex] as FolderFormState | undefined
  const folderErrorKeys = new Set(
    showErrors ? state.folders.filter((f) => errors.folders[f.key]).map((f) => f.key) : [],
  )

  function trySubmit() {
    if (errorCount > 0) {
      const badFolder = state.folders.find((f) => errors.folders[f.key])
      if (badFolder) setSelectedFolderKey(badFolder.key)
    }
    // Resolves every draft catalog into its inline `new` spec here, right
    // before it reaches the wire — `localCatalogs` is this editor's own
    // state, which `Workspace.tsx`'s `onSave` has no way to see.
    submit(errorCount, (finalState) => onSave(toCollectionPayload(finalState, localCatalogs)))
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
        {emptyFolders.length > 0 && (
          <span className="text-dim">
            {' '}
            · {pluralCount(emptyFolders.length, 'folder')} {emptyFolders.length === 1 ? 'has' : 'have'} no catalogs
          </span>
        )}
      </span>
    ) : emptyFolders.length > 0 ? (
      <span className="ed-status is-muted">
        No changes yet
        <span className="text-dim">
          {' '}
          · {pluralCount(emptyFolders.length, 'folder')} {emptyFolders.length === 1 ? 'has' : 'have'} no catalogs
        </span>
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
      docked="tv"
    >
      <div className="ed-container">
        <div className="ed ed-tv">
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
                {state.isPublic && privateReferencedCatalogs.length > 0 && (
                  <p className="field-error">
                    <Icon icon={TriangleAlert} size={16} className="text-dim" />
                    <span>
                      {pluralCount(privateReferencedCatalogs.length, 'catalog')} in here{' '}
                      {privateReferencedCatalogs.length === 1 ? "isn't" : "aren't"} shared on{' '}
                      {privateReferencedCatalogs.length === 1 ? 'its' : 'their'} own (
                      {privateReferencedCatalogs.map((c) => c.name).join(', ')}) — sharing this
                      collection shares its contents too.
                    </span>
                  </p>
                )}
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
                    : 'Only applies to tabbed grids.'}
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

            <div className="cr">
              <span className="cr-role type-eyebrow">Focus glow</span>
              <div className="cr-val ed-line">
                <Segmented
                  ariaLabel="Focus glow"
                  value={state.focusGlowEnabled ? 'on' : 'off'}
                  onChange={(value) => patch({ focusGlowEnabled: value === 'on' })}
                  options={[
                    { value: 'off', label: 'Off' },
                    { value: 'on', label: 'On' },
                  ]}
                />
                <span className="ed-note">Your TV's glow around a folder tile while it's focused.</span>
              </div>
            </div>

            <div className="cr is-head">
              <h2 className="cr-role type-eyebrow">Folders</h2>
              <div className="cr-val flex justify-end">
                <button
                  type="button"
                  className="btn-secondary btn-sm"
                  onClick={() =>
                    patchFolders((folders) => {
                      const folder = newFolder()
                      setSelectedFolderKey(folder.key)
                      return [...folders, folder]
                    })
                  }
                >
                  Add folder
                </button>
              </div>
            </div>

            {willDelete.length > 0 && (
              <RemovalWarning names={willDelete.map((f) => f.title.trim() || 'an untitled folder')} onUndo={undoRemoving} />
            )}

            {selectedFolder === undefined ? (
              <div className="cr-indent py-4">
                <p className="ed-note m-0">No folders yet. Add one, then put catalogs in it.</p>
              </div>
            ) : (
              <FolderTreeDnd
                folderKeys={state.folders.map((f) => f.key)}
                folderName={(key) => {
                  const index = state.folders.findIndex((f) => f.key === key)
                  const folder = state.folders[index]
                  return folder ? folderLabel(folder, index) : 'this folder'
                }}
                refName={(refKey) => {
                  const ref = state.folders.flatMap((f) => f.refs).find((r) => r.key === refKey)
                  const name = (ref && mergedOptionByID.get(ref.catalogID)?.name) ?? 'this catalog'
                  return ref?.genre ? `${name}, ${ref.genre}` : name
                }}
                onReorderFolders={reorderFolders}
                onReorderRefs={(folderKey, refKeys) => patchRefs(folderKey, (refs) => reorderRefs(refs, refKeys))}
              >
                <FolderTiles
                  folders={state.folders}
                  selectedKey={selectedFolder.key}
                  errorKeys={folderErrorKeys}
                  onSelect={setSelectedFolderKey}
                />
                <FolderDetail
                  // Remounted per folder so the Appearance row and the
                  // catalog picker start closed on each one.
                  key={selectedFolder.key}
                  folder={selectedFolder}
                  position={selectedIndex}
                  total={state.folders.length}
                  errors={showErrors ? errors.folders[selectedFolder.key] : undefined}
                  options={options}
                  optionByID={mergedOptionByID}
                  collectionID={collectionID}
                  usedInPlaces={usedInPlaces}
                  onChange={(update) => patchFolder(selectedFolder.key, update)}
                  onMove={(direction) => moveFolder(selectedFolder.key, direction)}
                  onRemove={() => patchFolders((folders) => folders.filter((f) => f.key !== selectedFolder.key))}
                  onAddRef={(catalogID) => addRef(selectedFolder.key, catalogID)}
                  onCopyRefIntoCollection={(catalogID) => {
                    const source = localOptionByID.get(catalogID)?.catalog ?? optionByID.get(catalogID)?.catalog
                    if (source) copyIntoCollection(selectedFolder.key, source)
                  }}
                  onRemoveRef={(refKey) => removeRef(selectedFolder.key, refKey)}
                  onSetRefGenre={(refKey, genre) => setRefGenre(selectedFolder.key, refKey, genre)}
                  onAddGenreRef={(refKey, genre) => addGenreRef(selectedFolder.key, refKey, genre)}
                  onMoveRef={(refKey, direction) => moveRef(selectedFolder.key, refKey, direction)}
                  onEditRef={editRef}
                  onCopyRef={(refKey, catalogID) => copyRefIntoCollection(selectedFolder.key, refKey, catalogID)}
                  onAddNewInCollection={() => startNewInCollection(selectedFolder.key)}
                />
              </FolderTreeDnd>
            )}
          </div>

          <CollectionPreview state={state} optionByID={mergedOptionByID} />
        </div>
      </div>

      <SaveError noun="collection" message={serverError} />

      {nestedCatalogID !== null &&
        (() => {
          // This modal only ever opens on a scoped catalog, real or draft —
          // `FolderCard.tsx`'s row has no quiet Edit for a listed one any
          // more — and every scoped catalog this editor knows about is
          // already in `localCatalogs` (seeded from `initialCatalogs`, grown
          // by copy/new-in-collection), so there is no library fallback here.
          const catalog = nestedCatalog
          if (!catalog || !nestedInitial) return null
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
                  initial={nestedInitial}
                  genres={genres}
                  certifications={certifications}
                  countryNames={countryNames}
                  languages={languages}
                  saving={!isDraftCatalogID(nestedCatalogID) && catalogMutations.update.isPending}
                  serverError={
                    isDraftCatalogID(nestedCatalogID)
                      ? null
                      : ((catalogMutations.update.error as Error | null)?.message ?? null)
                  }
                  onSave={saveNestedCatalog}
                  onRequestClose={closeNestedCatalog}
                  onDirtyChange={setNestedDirty}
                />
              </div>
              <ConfirmDialog
                open={confirmingNestedDiscard}
                title="Discard unsaved changes?"
                body={
                  <>
                    Your changes to <strong className="text-ink">{catalog.name}</strong> haven't
                    been saved. Leaving discards them.
                  </>
                }
                confirmLabel="Discard"
                cancelLabel="Keep editing"
                destructive
                onConfirm={discardNestedCatalog}
                onCancel={() => setConfirmingNestedDiscard(false)}
              />
            </Modal>
          )
        })()}

      <NewItemDialog
        open={namingNewFolderKey !== null}
        noun="catalog"
        label="Name"
        placeholder="Trending Sci-Fi"
        // Staged locally, not written to the DB until this collection's own
        // Save (see `createNewInCollection`) — there is nothing async here.
        saving={false}
        serverError={null}
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
        onClose={() => setNamingNewFolderKey(null)}
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
  return (
    <div className="cr">
      <span className="cr-role flex self-start justify-end pt-0.5 max-[640px]:justify-start">
        <Icon icon={TriangleAlert} size={16} className="text-danger" />
      </span>
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span className="text-[14px] leading-[20px]">
          Saving deletes {names.length === 1 ? 'the folder' : 'the folders'} {joinQuoted(names)}.
          Copies others have taken keep theirs.
        </span>
        <button type="button" onClick={onUndo} className="btn-quiet h-auto px-0 text-[13px]">
          Undo
        </button>
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
