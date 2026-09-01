import { useEffect, useMemo, useState } from 'react'
import { Checkbox, Field, Select, TextInput } from '@/components/fields'
import { EditorShell } from '@/features/builder/EditorShell'
import { plural } from '@/lib/plural'
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
  const [state, setState] = useState<CollectionFormState>(baseline)
  const [showErrors, setShowErrors] = useState(false)

  useEffect(() => {
    setState(baseline)
    setShowErrors(false)
  }, [baseline])

  const errors = useMemo(
    () => validateCollectionForm(state, accessibleIDs),
    [state, accessibleIDs],
  )
  const errorCount = countErrors(errors)
  const willDelete = useMemo(() => removedFolders(baseline, state), [baseline, state])
  const dirty = !isSameCollection(baseline, state)
  useEffect(() => onDirtyChange(dirty), [dirty, onDirtyChange])

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

  function submit() {
    setShowErrors(true)
    if (errorCount > 0) return
    onSave(state)
  }

  const eyebrow = mode === 'edit' ? 'Edit collection' : 'Duplicate collection'

  return (
    <EditorShell
      eyebrow={eyebrow}
      title={state.title.trim() || 'Untitled collection'}
      kind="collection"
      // A duplicate is born yours, and so is a new one — the only editable
      // collection is one you own.
      owned
      onRequestClose={onRequestClose}
      onDuplicate={onDuplicate}
      onDelete={onDelete}
      footer={
        <>
          {showErrors && errorCount > 0 && (
            <span className="type-data text-danger mr-auto text-[10.5px]">
              Fix the highlighted {plural(errorCount, 'field')}.
            </span>
          )}
          <button type="button" onClick={onRequestClose} className="btn-ghost">
            Cancel
          </button>
          <button type="button" onClick={submit} disabled={saving} className="btn-primary">
            {saving ? 'Saving…' : mode === 'edit' ? 'Save changes' : 'Create collection'}
          </button>
        </>
      }
    >
          {droppedRefs.length > 0 && (
            <p className="type-data text-dim border-series mb-6 border-l-2 pl-3 text-[11px] leading-[1.45]">
              {droppedRefs.length === 1 ? 'One catalog was' : `${droppedRefs.length} catalogs were`}{' '}
              left out of this copy — no longer shared with you.
            </p>
          )}

          {/* --- collection ------------------------------------------------ */}
          <div className="grid gap-x-8 gap-y-6 md:grid-cols-[minmax(0,280px)_minmax(0,1fr)]">
            <div className="flex flex-col gap-6">
              <Field label="Title" error={showErrors ? errors.title : undefined}>
                <TextInput
                  value={state.title}
                  onChange={(title) => patch({ title })}
                  placeholder="Saturday night"
                  invalid={Boolean(showErrors && errors.title)}
                />
              </Field>

              <Checkbox
                checked={state.isPublic}
                onChange={(isPublic) => patch({ isPublic })}
                label="Share with the community"
                hint="Others can add it to their own home screen."
              />
            </div>

            <div className="flex flex-col gap-6">
              {/* No `placeholder`: these three are the whole of `view_mode`,
                  and an empty option would be a fourth choice the type has no
                  value for. */}
              <Field
                label="View mode"
                tip="How a folder in this collection opens. Tabbed Grids puts one tab per catalog above a grid of its titles; Rows stacks each catalog as its own scrolling row. Follow layout leaves the choice to whatever the app is set to."
              >
                <Select
                  value={state.viewMode}
                  onChange={(viewMode) => patch({ viewMode: viewMode as CollectionViewMode })}
                  options={VIEW_MODES.map((value) => ({
                    value,
                    label: VIEW_MODE_LABELS[value],
                  }))}
                />
              </Field>

              <div className="flex flex-col gap-4">
                <Checkbox
                  checked={state.pinToTop}
                  onChange={(pinToTop) => patch({ pinToTop })}
                  label="Show first on the home screen"
                />
                {/* Only tabbed grids have tabs to add one to. The value is
                    left alone while it is greyed — it is still what this
                    collection is set to, and applies again the moment the view
                    mode goes back. */}
                <Checkbox
                  checked={state.showAllTab}
                  onChange={(showAllTab) => patch({ showAllTab })}
                  disabled={state.viewMode !== 'TABBED_GRID'}
                  label={'Show an "All" tab'}
                  hint={
                    state.viewMode === 'TABBED_GRID'
                      ? 'Adds a first tab holding every catalog in a folder at once.'
                      : `Only Tabbed Grids has tabs`
                  }
                />
              </div>

              <Field
                label="Background image"
                tip="Shown behind this collection when someone opens it. Paste a link to an image."
              >
                <TextInput
                  value={state.backdropImageURL}
                  onChange={(backdropImageURL) => patch({ backdropImageURL })}
                  placeholder="https://…"
                />
              </Field>
            </div>
          </div>

          {/* --- folders ---------------------------------------------------- */}
          <div className="border-line mt-8 flex items-center gap-3 border-t pt-6">
            <span className="type-eyebrow flex-1">
              Folders{' '}
              <span className="type-data text-dimmer normal-case">({state.folders.length})</span>
            </span>
            <button
              type="button"
              className="btn-ghost"
              onClick={() => patchFolders((folders) => [...folders, newFolder()])}
            >
              Add folder
            </button>
          </div>
          {state.folders.length === 0 ? (
            <p className="type-data text-dimmer m-0 py-2 text-[11px]">
              No folders yet. Add one, then put catalogs in it.
            </p>
          ) : (
            <FolderTreeDnd
              folderKeys={state.folders.map((f) => f.key)}
              onReorderFolders={reorderFolders}
              onReorderRefs={(folderKey, catalogIDs) => patchFolder(folderKey, { catalogIDs })}
            >
              <ul className="m-0 flex list-none flex-col gap-3 p-0">
                {state.folders.map((folder, index) => (
                  <FolderCard
                    key={folder.key}
                    folder={folder}
                    position={index}
                    total={state.folders.length}
                    errors={showErrors ? errors.folders[folder.key] : undefined}
                    options={options}
                    optionByID={optionByID}
                    onChange={(update) => patchFolder(folder.key, update)}
                    onRemove={() =>
                      patchFolders((folders) => folders.filter((f) => f.key !== folder.key))
                    }
                    onAddRef={(catalogID) => addRef(folder.key, catalogID)}
                    onRemoveRef={(catalogID) => removeRef(folder.key, catalogID)}
                  />
                ))}
              </ul>
            </FolderTreeDnd>
          )}

          <CollectionPreview state={state} optionByID={optionByID} />

          {willDelete.length > 0 && (
            <p className="type-data text-danger border-danger mt-6 border-l-2 pl-3 text-[11px] leading-[1.45]">
              Saving deletes{' '}
              {willDelete.map((f) => f.title.trim() || 'an untitled folder').join(', ')} and
              everything inside, for everyone using this collection. Cancel to keep{' '}
              {willDelete.length === 1 ? 'it' : 'them'}.
            </p>
          )}

          {serverError && (
            <p className="type-data text-danger border-danger mt-6 border-l-2 pl-3 text-[11px] leading-[1.45]">
              Couldn't save this collection: {serverError}
            </p>
          )}
    </EditorShell>
  )
}
