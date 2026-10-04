import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import type {
  Catalog,
  CatalogType,
  CertificationsByCountry,
  CollectionPayload,
  Genre,
  Language,
} from '@/api'
import { CATALOG_PROVIDER } from '@/api'
import type { SignStep } from '@/components/PaneSign'
import { FieldError, TextInput } from '@/components/fields'
import { EditorFooter } from '@/features/builder/EditorFooter'
import { EditorShell } from '@/features/builder/EditorShell'
import { NewItemDialog } from '@/features/builder/NewItemDialog'
import { useEditorForm } from '@/features/builder/useEditorForm'
import {
  emptyForm,
  formFromCatalog,
  toPayload as toCatalogPayload,
} from '@/features/catalogs/catalogForm'
import type { CatalogFormState } from '@/features/catalogs/catalogForm'
import type { CountryLookup } from '@/features/catalogs/countries'
import { CatalogTypeField } from '@/features/catalogs/fields'
import type { GenreLookups } from '@/features/library/useLibrary'
import { andList } from '@/lib/list'
import { moveByOne, orderByKeys } from '@/lib/order'
import { pluralCount } from '@/lib/plural'
import { CollectionAppearance } from './CollectionAppearance'
import { CollectionPreview } from './CollectionPreview'
import { FolderDetail } from './FolderDetail'
import { FolderTiles, FolderTreeDnd } from './FolderTree'
import {
  DRAFT_ID_PREFIX,
  countErrors,
  folderLabel,
  isDraftCatalogID,
  hasRef,
  isSameCollection,
  newFolder,
  newRef,
  previewFromForm,
  removedFolders,
  toCollectionPayload,
  validateCollectionForm,
  withCatalogEdit,
  type CollectionFormState,
  type FolderFormState,
  type FolderRefState,
} from './collectionForm'
import { errorRoleLabels, nestedCatalogForm, withGenreRef, withRefs } from './folderEdits'
import { NestedCatalogEditor } from './NestedCatalogEditor'
import { StagedNote } from './StagedNote'
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
    collection_id: seed.collectionID,
    created_at: '',
    updated_at: '',
    publication: null,
    subscription: null,
  }
}

interface CollectionEditorProps {
  /** The form as the row stands. A new identity re-seeds the editor (see
   *  `useEditorForm`), so callers hand over a stable object. */
  initial: CollectionFormState
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
  /** The × on the sign, Escape and the footer's Close — see `EditorShell`. */
  onRequestClose: () => void
  /** This collection's own row actions, carried in the header below `lg`.
   *  Absent until the library lists a row that was just created. */
  onDuplicate?: () => void
  onDelete?: () => void
  onDirtyChange: (dirty: boolean) => void
  /** This collection's own server id: what a copied or new catalog is scoped
   *  to. */
  collectionID: string
  /** Every catalog this collection's folders already reference, listed or
   *  scoped. A duplicated collection arrives here with its own fresh scoped
   *  copies already populated (`Workspace.tsx`'s `EditorTarget.initialCatalogs`
   *  override). Seeds `localCatalogs`. */
  initialCatalogs: Catalog[]
  genres: { movie: Genre[]; tv: Genre[] }
  /** Same genres, shaped for `buildRefOptions` rather than the nested
   *  `CatalogEditor`'s own picker — building `RefOption`s for a freshly
   *  scoped catalog needs the lookup form, `Workspace`'s own already computed
   *  from this. */
  genreLookups: GenreLookups
  certifications: { movie: CertificationsByCountry; tv: CertificationsByCountry }
  countryNames: CountryLookup
  languages: Language[]
  /** How many folders across every owned collection reference a catalog —
   *  computed in `Workspace`, which is the level that has the whole library.
   *  Meaningful only for a listed catalog. */
  usedInFolders: (catalogID: string) => number
  /** Its next step with Community, as the sign's button. */
  sharingStep?: SignStep
  /** Its sharing stickers, on the sign beside the kind. */
  sharingBadges?: ReactNode
}

/**
 * Edit a collection, folders and catalog refs included, filling the builder's
 * right pane. Always a saved row: a collection is titled into existence
 * before this editor opens, and Duplicate is one server call
 * (`DuplicateCollection`) whose finished copy opens here like any other row.
 *
 * **The whole tree, one save.** `POST`/`PUT` replace the collection, its
 * folders, every folder's refs and every edit to its scoped catalogs in a
 * single transaction, so this editor has one dirty state and one Save button
 * rather than a save per folder or per catalog.
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
 * this collection** (the same, named first). The last two are staged as drafts
 * scoped to this collection and written by its Save.
 *
 * **This editor keeps its own catalog registry** (`localCatalogs`), seeded
 * from `initialCatalogs` and grown by every scoped create/edit it makes —
 * `GET /api/p/{i}/catalogs` is listed catalogs only, so the library-wide
 * `options`/`optionByID`/`accessibleIDs` this editor is handed never contain
 * a scoped one. Folder rows render from this merged registry, never the
 * library alone, which is what makes a scoped catalog show at all.
 *
 * **Quiet Edit opens a catalog one level down**, and its Done writes nothing:
 * it stages the change in this editor's own form (`catalogEdits`), which this
 * collection's Save sends as `catalog_edits`. It sits in a `Modal` layered
 * over this editor rather than a second pane — the builder's pane holds one
 * occupant (`EditorShell`'s own doc comment), so a second, real editor has to
 * be a modal, not a stack. `CollectionEditor` stays mounted underneath, so
 * this editor's own unsaved folder edits survive the round trip.
 *
 * **Every row this editor opens is the profile's own and editable.** A
 * collection added from Community opens as a view instead
 * (`FromCommunityView`). Its next step with Community is `sharingStep`, which
 * the pane builds and the sign carries as its one button.
 *
 * **A staged Move to library has its own Undo.** Once staged, the catalog
 * reads as listed in every folder, which offers no Edit to reopen it, so the
 * standing note naming it is the way back short of discarding the whole form.
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
  genres,
  genreLookups,
  certifications,
  countryNames,
  languages,
  usedInFolders,
  sharingStep,
  sharingBadges,
}: CollectionEditorProps) {
  const baseline = initial
  const { state, setState, dirty, showErrors, submit } = useEditorForm(
    baseline,
    isSameCollection,
    onDirtyChange,
  )

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
  // Every scoped catalog as this editor opened it, before any staged edit —
  // what `withCatalogEdit` compares a nested save against, so an edit made
  // and then undone leaves no pending edit behind.
  const [savedCatalogs] = useState<ReadonlyMap<string, Catalog>>(
    () => new Map(initialCatalogs.map((c) => [c.id, c])),
  )

  const localOptions = useMemo(
    () => buildRefOptions([...localCatalogs.values()], genreLookups),
    [localCatalogs, genreLookups],
  )
  const localOptionByID = useMemo(() => indexRefOptions(localOptions), [localOptions])
  const mergedOptionByID = useMemo(
    () => new Map([...optionByID, ...localOptionByID]),
    [optionByID, localOptionByID],
  )
  const preview = useMemo(() => previewFromForm(state, mergedOptionByID), [state, mergedOptionByID])
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
    () => [...localCatalogs.values()].filter((c) => c.collection_id === collectionID && !isDraftCatalogID(c.id)),
    [localCatalogs, collectionID],
  )
  const catalogsWillDelete = useMemo(
    () => scopedHere.filter((c) => !state.folders.some((f) => f.refs.some((ref) => ref.catalogID === c.id))),
    [scopedHere, state.folders],
  )

  // Legal to save — the server accepts a folder with no catalog refs — but it
  // shows nothing in Nuvio, so it's surfaced as a standing warning
  // alongside the deletion counts rather than left to the folder's own,
  // possibly-never-opened panel note.
  const emptyFolders = useMemo(() => state.folders.filter((f) => f.refs.length === 0), [state.folders])

  // Catalogs the next Save moves out of this collection into the library.
  const movingToLibrary = Object.entries(state.catalogEdits)
    .filter(([, edit]) => edit.moveToLibrary)
    .map(([id]) => localCatalogs.get(id))
    .filter((catalog) => catalog !== undefined)

  const errors = useMemo(
    () => validateCollectionForm(state, mergedAccessibleIDs),
    [state, mergedAccessibleIDs],
  )
  const errorCount = countErrors(errors)
  const willDelete = useMemo(() => removedFolders(baseline, state), [baseline, state])

  // The nested catalog editor — a modal layered over this pane, not a second
  // occupant of it. `null` means closed.
  const [nestedCatalogID, setNestedCatalogID] = useState<string | null>(null)
  const nestedCatalog = nestedCatalogID === null ? undefined : localCatalogs.get(nestedCatalogID)
  // Keyed on the catalog object, which only changes when `rememberCatalog`
  // replaces it: a fresh form on every render of this editor would re-seed the
  // nested one (see `useEditorForm`) and drop its edits when a save fails.
  const nestedInitial = useMemo(() => nestedCatalogForm(nestedCatalog), [nestedCatalog])
  // "New inside this collection" is named first, same two-step as the main
  // library's own "New catalog" — see Workspace's `createBareCatalog`.
  const [namingNewFolderKey, setNamingNewFolderKey] = useState<string | null>(null)
  const [newCatalogType, setNewCatalogType] = useState<CatalogType>('movie')

  function editRef(catalogID: string) {
    setNestedCatalogID(catalogID)
  }

  /** The nested modal's way out, once its own guard has let it go. */
  function closeNestedCatalog() {
    setNestedCatalogID(null)
  }

  /** Applies the nested editor's save locally and writes nothing: the
   *  catalog (only ever a scoped one — a listed ref has no quiet Edit here,
   *  see `RefRow.tsx`) is replaced in `localCatalogs` so every folder
   *  shows the change. A draft needs nothing more, since this collection's
   *  own Save resolves it into an inline `new` spec (`toCollectionPayload`);
   *  a real row also gets a pending edit in the form (`withCatalogEdit`),
   *  which the same Save sends as `catalog_edits`. */
  function saveNestedCatalog(formState: CatalogFormState) {
    if (nestedCatalogID === null) return
    const catalog = localCatalogs.get(nestedCatalogID)
    if (!catalog) return
    const payload = toCatalogPayload(formState)
    rememberCatalog({
      ...catalog,
      type: payload.type,
      name: payload.name,
      params: payload.params,
      collection_id: formState.collectionID,
    })
    if (!isDraftCatalogID(catalog.id)) {
      const saved = savedCatalogs.get(catalog.id) ?? catalog
      setState((previous) => withCatalogEdit(previous, saved, formState))
    }
    setNestedCatalogID(null)
  }

  /** Undo on a staged Move to library: the catalog goes back into this
   *  collection in `localCatalogs`, and its pending edit keeps whatever else
   *  the nested save changed, or goes once nothing else is left
   *  (`withCatalogEdit`). */
  function undoMoveToLibrary(catalogID: string) {
    const catalog = localCatalogs.get(catalogID)
    const saved = savedCatalogs.get(catalogID)
    if (!catalog || !saved) return
    const restored = { ...catalog, collection_id: saved.collection_id }
    rememberCatalog(restored)
    setState((previous) => withCatalogEdit(previous, saved, formFromCatalog(restored)))
  }

  /** A fresh scoped copy of `catalogID`, staged locally with the source's
   *  exact saved values (never re-derived through the form, so its `params`
   *  string round-trips byte for byte), and its draft id. Not written to the
   *  DB until this collection's own Save — see `draftCatalog`'s doc comment.
   *  The source is looked up in the merged registry: a catalog linked this
   *  session is a library one this editor's own registry never had reason to
   *  remember. */
  function stageCopy(catalogID: string): string | undefined {
    const source = mergedOptionByID.get(catalogID)?.catalog
    if (!source) return undefined
    const draft = draftCatalog({ type: source.type, name: source.name, params: source.params, collectionID })
    rememberCatalog(draft)
    return draft.id
  }

  /** "Copy" from the Add-catalogs picker: the staged copy as a new ref. */
  function copyIntoCollection(folderKey: string, catalogID: string) {
    const draftID = stageCopy(catalogID)
    if (draftID) addRef(folderKey, draftID)
  }

  /** "Copy into this collection" on a listed catalog's own row: the staged
   *  copy replaces that ref in place rather than adding a second one, so the
   *  folder's order doesn't change. Only this ref moves to the copy, keeping
   *  its genre; another ref to the same listed catalog under a different
   *  genre stays linked. */
  function copyRefIntoCollection(folderKey: string, refKey: string, catalogID: string) {
    const draftID = stageCopy(catalogID)
    if (!draftID) return
    patchRefs(folderKey, (refs) => refs.map((ref) => (ref.key === refKey ? { ...ref, catalogID: draftID } : ref)))
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
    if (namingNewFolderKey === null) return
    const folderKey = namingNewFolderKey
    const payload = toCatalogPayload({ ...emptyForm(newCatalogType, collectionID), name })
    const draft = draftCatalog({ type: payload.type, name: payload.name, params: payload.params, collectionID })
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
    patchFolders((folders) => orderByKeys(folders, orderedKeys, (f) => f.key))
  }

  function moveFolder(key: string, direction: -1 | 1) {
    patchFolders((folders) =>
      orderByKeys(folders, moveByOne(folders.map((f) => f.key), key, direction), (f) => f.key),
    )
  }

  function reorderRefs(folderKey: string, orderedKeys: string[]) {
    patchRefs(folderKey, (refs) => orderByKeys(refs, orderedKeys, (ref) => ref.key))
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
    patchFolders((folders) => folders.map((f) => withGenreRef(f, folderKey, refKey, genre)))
  }

  function patchRefs(folderKey: string, update: (refs: FolderRefState[]) => FolderRefState[]) {
    patchFolders((folders) => folders.map((f) => withRefs(f, folderKey, update)))
  }

  function removeRef(folderKey: string, refKey: string) {
    patchRefs(folderKey, (refs) => refs.filter((ref) => ref.key !== refKey))
  }

  function setRefGenre(folderKey: string, refKey: string, genre: string) {
    patchRefs(folderKey, (refs) => refs.map((ref) => (ref.key === refKey ? { ...ref, genre } : ref)))
  }

  function moveRef(folderKey: string, refKey: string, direction: -1 | 1) {
    patchRefs(folderKey, (refs) =>
      orderByKeys(refs, moveByOne(refs.map((ref) => ref.key), refKey, direction), (ref) => ref.key),
    )
  }

  /** A new, untitled folder at the end, selected so its panel opens. */
  function addFolder() {
    const folder = newFolder()
    setSelectedFolderKey(folder.key)
    patchFolders((folders) => [...folders, folder])
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

  // Resolves every draft catalog into its inline `new` spec here, right
  // before it reaches the wire — `localCatalogs` is this editor's own
  // state, which `Workspace.tsx`'s `onSave` has no way to see.
  function save(finalState: CollectionFormState) {
    onSave(toCollectionPayload(finalState, localCatalogs))
  }

  function trySubmit() {
    if (errorCount > 0) {
      const badFolder = state.folders.find((f) => errors.folders[f.key])
      if (badFolder) setSelectedFolderKey(badFolder.key)
    }
    submit(errorCount, save)
  }

  const roleLabels = showErrors ? errorRoleLabels(errors, state.folders) : []

  // What the save bar adds after its status: what the next Save deletes, and
  // folders that would show nothing — the last one whether or not anything
  // has changed.
  const statusNotes = [
    ...(dirty && willDelete.length > 0 ? [`${pluralCount(willDelete.length, 'folder')} will be deleted`] : []),
    ...(dirty && catalogsWillDelete.length > 0
      ? [`${pluralCount(catalogsWillDelete.length, 'catalog')} will be deleted`]
      : []),
    ...(emptyFolders.length > 0
      ? [`${pluralCount(emptyFolders.length, 'folder')} ${emptyFolders.length === 1 ? 'has' : 'have'} no catalogs`]
      : []),
  ]

  // One standing note per catalog the next Save moves into the library.
  const movingNotes = movingToLibrary.map((catalog) => (
    <StagedNote key={catalog.id} tone="neutral" onUndo={() => undoMoveToLibrary(catalog.id)}>
      Saving moves “{catalog.name}” out of this collection and into your library.
    </StagedNote>
  ))

  return (
    <EditorShell
      purpose="Edit collection"
      tone="collection"
      badges={
        <>
          <span className="stk stk-neutral">Collection</span>
          {sharingBadges}
        </>
      }
      step={sharingStep}
      title={state.title.trim() || 'Untitled collection'}
      onRequestClose={onRequestClose}
      onDuplicate={onDuplicate}
      onDelete={onDelete}
      footer={
        <EditorFooter
          noun="collection"
          saving={saving}
          errorCount={errorCount}
          errorLabels={roleLabels}
          dirty={dirty}
          notes={statusNotes}
          onCancel={onRequestClose}
          onSubmit={trySubmit}
          saveLabel="Save"
          saveError={serverError}
        />
      }
    >
      <div className="ed-container">
        <div className="ed ed-preview">
          <div className="ed-form">
            <div className="setting">
              <label htmlFor="col-title" className="setting-label type-label">
                Title
              </label>
              <div className="setting-value">
                <TextInput
                  id="col-title"
                  value={state.title}
                  onChange={(title) => patch({ title })}
                  placeholder="Saturday night"
                  invalid={Boolean(showErrors && errors.title)}
                />
                {showErrors && errors.title && <FieldError>{errors.title}</FieldError>}
              </div>
            </div>

            <div className="setting is-head">
              <h2 className="setting-label type-label">Folders</h2>
              <div className="setting-value flex justify-end">
                <button type="button" className="btn-secondary btn-sm" onClick={addFolder}>
                  Add folder
                </button>
              </div>
            </div>

            {willDelete.length > 0 && (
              <StagedNote tone="danger" onUndo={undoRemoving}>
                Saving deletes {willDelete.length === 1 ? 'the folder' : 'the folders'}{' '}
                {andList(willDelete.map((f) => `“${f.title.trim() || 'an untitled folder'}”`))}. People who
                added it keep theirs.
              </StagedNote>
            )}

            {movingNotes}

            {selectedFolder === undefined ? (
              <div className="py-4">
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
                onReorderRefs={reorderRefs}
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
                  usedInFolders={usedInFolders}
                  onChange={(update) => patchFolder(selectedFolder.key, update)}
                  onMove={(direction) => moveFolder(selectedFolder.key, direction)}
                  onRemove={() => patchFolders((folders) => folders.filter((f) => f.key !== selectedFolder.key))}
                  onAddRef={(catalogID) => addRef(selectedFolder.key, catalogID)}
                  onCopyRefIntoCollection={(catalogID) => copyIntoCollection(selectedFolder.key, catalogID)}
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

            <CollectionAppearance state={state} onChange={patch} />
          </div>

          <CollectionPreview collection={preview} />
        </div>
      </div>

      {/* Only ever a scoped catalog, real or draft — `RefRow` has no quiet
          Edit for a listed one — and every scoped catalog this editor knows
          about is already in `localCatalogs` (seeded from `initialCatalogs`,
          grown by copy/new-in-collection), so there is no library fallback. */}
      {nestedCatalog && nestedInitial && (
        <NestedCatalogEditor
          catalog={nestedCatalog}
          initial={nestedInitial}
          genres={genres}
          certifications={certifications}
          countryNames={countryNames}
          languages={languages}
          onSave={saveNestedCatalog}
          onClose={closeNestedCatalog}
        />
      )}

      <NewItemDialog
        open={namingNewFolderKey !== null}
        noun="catalog"
        label="Name"
        placeholder="Trending Sci-Fi"
        // Staged locally, not written to the DB until this collection's own
        // Save (see `createNewInCollection`) — there is nothing async here.
        saving={false}
        serverError={null}
        extra={<CatalogTypeField value={newCatalogType} onChange={setNewCatalogType} />}
        onCreate={createNewInCollection}
        onClose={() => setNamingNewFolderKey(null)}
      />
    </EditorShell>
  )
}
