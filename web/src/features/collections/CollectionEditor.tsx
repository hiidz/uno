import { useEffect, useMemo, useState } from 'react'
import { Checkbox, Field, Select, TextInput } from '@/components/fields'
import { EditorShell } from '@/features/builder/EditorShell'
import { CollectionPreview } from './CollectionPreview'
import { FolderCard, FolderTreeDnd } from './FolderCard'
import {
  VIEW_MODES,
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
  onRequestClose: () => void
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
      footer={
        <>
          {showErrors && errorCount > 0 && (
            <span className="type-data text-danger mr-auto text-[10.5px]">
              Fix the highlighted {errorCount === 1 ? 'field' : 'fields'}.
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
              left out of this copy — {droppedRefs.length === 1 ? 'it is' : 'they are'} no longer
              shared with you. Everything else came across.
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
                hint="Anyone signed in can add it to their own home screen."
              />
            </div>

            <div className="flex flex-col gap-6">
              <Field
                label="View mode"
                hint="How items are displayed in Nuvio. Leave unset to use Nuvio's default."
              >
                <Select
                  value={state.viewMode}
                  onChange={(viewMode) => patch({ viewMode: viewMode as CollectionViewMode })}
                  placeholder="Default"
                  options={VIEW_MODES.map((value) => ({
                    value,
                    label: value === 'TABBED_GRID' ? 'Grid' : value === 'ROWS' ? 'List' : 'Follow layout',
                  }))}
                />
              </Field>

              <div className="flex flex-col gap-4">
                <Checkbox
                  checked={state.pinToTop}
                  onChange={(pinToTop) => patch({ pinToTop })}
                  label="Pin to the front of the tab strip"
                  hint="Shows this collection first, keeping pinned collections in your chosen order."
                />
                <Checkbox
                  checked={state.showAllTab}
                  onChange={(showAllTab) => patch({ showAllTab })}
                  label={'Show an "All" tab'}
                  hint="A first tab merging every folder in this collection."
                />
              </div>

              <Field
                label="Backdrop image URL"
                hint="Displayed behind the collection in Nuvio when it's opened."
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
          <p className="type-data text-dimmer mt-2 mb-4 max-w-[38rem] text-[11px] leading-[1.45]">
            Folders become tabs in Nuvio. Drag to reorder them. Within each folder, reorder catalogs the same way.
          </p>

          {state.folders.length === 0 ? (
            <p className="type-data text-dimmer m-0 py-2 text-[11px]">
              No folders yet. Add one to create a tab in Nuvio, then add catalogs to it.
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
              {willDelete.map((f) => f.title.trim() || 'an untitled folder').join(', ')} — the{' '}
              {willDelete.length === 1 ? 'folder and everything in it' : 'folders and everything in them'}{' '}
              {willDelete.length === 1 ? 'is' : 'are'} removed for everyone using this collection.
              Cancel to keep {willDelete.length === 1 ? 'it' : 'them'}.
            </p>
          )}

          {serverError && (
            <p className="type-data text-danger border-danger mt-6 border-l-2 pl-3 text-[11px] leading-[1.45]">
              The server rejected this collection: {serverError}
            </p>
          )}
    </EditorShell>
  )
}
