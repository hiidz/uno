import { useMemo, useState } from 'react'
import { TriangleAlert } from 'lucide-react'
import { moveByOne } from '@/components/dnd'
import { Segmented, Switch, TextInput } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { EditorFooter, SaveError } from '@/features/builder/EditorFooter'
import { EditorShell } from '@/features/builder/EditorShell'
import { useEditorForm } from '@/features/builder/useEditorForm'
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
  type BuilderMode,
  type CollectionFormState,
  type CollectionViewMode,
  type FolderFormState,
} from './collectionForm'
import type { RefOption } from './refs'

/**
 * Create / edit / duplicate a collection, folders and catalog refs included,
 * filling the builder's right pane.
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
 * **Ships against today's reference-based `folder_catalogs` model, not
 * DESIGN.md's folder-catalogs-as-private-copies** — that data-model change is
 * explicitly out of scope for this phase (see the migration plan's §2). Where
 * the two disagree, this editor states today's real behaviour rather than
 * copy that assumes the other model: a folder-catalog row carries no quiet
 * Edit (see `FolderCard`'s `RefRow`), and the copy-from-library note says a
 * ref stays linked rather than claiming a copy is made.
 *
 * **The read-only view of an imported collection (DESIGN.md's own spec for
 * this editor) isn't built here**, for the same two reasons Phase 4 gave for
 * the catalog editor: it needs the owner's `@handle`, which is paused work,
 * and today opening a community row has always meant an immediately-editable
 * duplicate — swapping that for a read-only screen plus a Duplicate button is
 * strictly more steps to the one thing that screen lets you do.
 */
export function CollectionEditor({
  mode,
  initial,
  droppedRefs,
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
}: {
  mode: BuilderMode
  initial: CollectionFormState | null
  /** Refs the seed dropped because this profile can't reference them — only
   *  ever non-empty when duplicating. See `formFromCollection`. */
  droppedRefs: string[]
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
}) {
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

  const errors = useMemo(
    () => validateCollectionForm(state, accessibleIDs),
    [state, accessibleIDs],
  )
  const errorCount = countErrors(errors)
  const willDelete = useMemo(() => removedFolders(baseline, state), [baseline, state])

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
      </span>
    ) : (
      <span className="ed-status is-muted">No changes yet</span>
    )

  const eyebrow = mode === 'edit' ? 'Edit collection' : 'Duplicate collection'

  return (
    <EditorShell
      eyebrow={eyebrow}
      title={state.title.trim() || 'Untitled collection'}
      onRequestClose={onRequestClose}
      onDuplicate={onDuplicate}
      onDelete={onDelete}
      footer={
        <EditorFooter
          mode={mode}
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
      {droppedRefs.length > 0 && (
        <p className="type-data text-dim border-line-hi mb-6 border-l-2 pl-3 text-[11px] leading-[1.45]">
          {droppedRefs.length === 1 ? 'One catalog was' : `${droppedRefs.length} catalogs were`}{' '}
          left out of this copy — no longer shared with you.
        </p>
      )}

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
                      ? 'Shared, so anyone can import it'
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
              <RemovalWarning names={willDelete.map((f) => f.title.trim() || 'an untitled folder')} shared={baseline.isPublic} onUndo={undoRemoving} />
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
                      optionByID={optionByID}
                      onChange={(update) => patchFolder(folder.key, update)}
                      onMove={(direction) => moveFolder(folder.key, direction)}
                      onRemove={() => {
                        patchFolders((folders) => folders.filter((f) => f.key !== folder.key))
                        setOpenFolderKey((current) => (current === folder.key ? null : current))
                      }}
                      onAddRef={(catalogID) => addRef(folder.key, catalogID)}
                      onRemoveRef={(catalogID) => removeRef(folder.key, catalogID)}
                      onMoveRef={(catalogID, direction) => moveRef(folder.key, catalogID, direction)}
                    />
                  ))}
                </ul>
              </FolderTreeDnd>
            )}
          </div>

          <CollectionPreview state={state} optionByID={optionByID} />
        </div>
      </div>

      <SaveError noun="collection" message={serverError} />
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
  shared,
  onUndo,
}: {
  names: string[]
  /** The collection's own sharing state — a removed folder isn't shared or
   *  unshared on its own, the collection around it is. */
  shared: boolean
  onUndo: () => void
}) {
  const noun = names.length === 1 ? 'the folder' : 'the folders'
  const pronoun = names.length === 1 ? 'it' : 'them'

  return (
    <div className="border-line mt-6 grid grid-cols-[16px_minmax(0,1fr)] gap-x-2 border-y py-4">
      <Icon icon={TriangleAlert} size={16} className="text-danger mt-0.5" />
      <div className="flex flex-wrap items-baseline gap-x-1.5 gap-y-1.5">
        <span className="text-[14px] leading-[20px]">
          The next Save will delete {noun} {joinQuoted(names)}.{' '}
          {shared
            ? `It goes for you and everyone who imported this collection.`
            : `It isn’t shared, so only you lose ${pronoun}.`}{' '}
          Nothing is gone yet.
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
