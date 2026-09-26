import { useId, useMemo, useState } from 'react'
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { ChevronDown, ChevronLeft, ChevronRight, Copy, Plus } from 'lucide-react'
import type { TileShape } from '@/api'
import { RowIconButton } from '@/components/dnd'
import { FieldError, Segmented, TextInput } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { ordinal } from '@/lib/ordinal'
import { CatalogRefPicker } from './CatalogRefPicker'
import { TILE_SHAPES, folderLabel, type FolderErrors, type FolderFormState } from './collectionForm'
import { refDragID } from './folderDnd'
import { RefRow } from './RefRow'
import type { RefOption } from './refs'

const SHAPE_LABEL: Record<TileShape, string> = {
  POSTER: 'poster',
  LANDSCAPE: 'landscape',
  SQUARE: 'square',
}

/** One line under the open folder's name, and the closed Appearance row. */
function appearanceSummary(folder: FolderFormState): string {
  const parts = [`${SHAPE_LABEL[folder.tileShape]} tile`, folder.hideTitle ? 'title hidden' : 'title shown']
  if (folder.coverImageURL.trim()) parts.push('cover image')
  else if (folder.coverEmoji.trim()) parts.push(`${folder.coverEmoji.trim()} cover`)
  if (folder.focusGIFEnabled && folder.focusGIFURL.trim()) parts.push('focus GIF')
  if (folder.heroBackdropURL.trim() || folder.heroVideoURL.trim() || folder.titleLogoURL.trim()) {
    parts.push('Modern Home hero')
  }
  return parts.join(' · ')
}

/**
 * The selected folder: a heading naming it and where it sits in the row,
 * then its title, its catalogs, and its appearance settings folded behind one
 * summarised row — the catalogs are what a folder is opened for, so they come
 * before the settings that are rarely touched.
 */
export function FolderDetail({
  folder,
  position,
  total,
  errors,
  onChange,
  onMove,
  onRemove,
  ...catalogs
}: {
  folder: FolderFormState
  position: number
  total: number
  /** Undefined until the user has tried to save — same rule as the catalog
   *  builder, which withholds errors until submit. */
  errors: FolderErrors | undefined
  onChange: (update: Partial<FolderFormState>) => void
  onMove: (direction: -1 | 1) => void
  onRemove: () => void
} & Omit<FolderCatalogsProps, 'folder' | 'errors'>) {
  const idBase = useId()
  const [appearanceOpen, setAppearanceOpen] = useState(false)
  const label = folderLabel(folder, position)

  return (
    <section className="fold-detail" aria-label={`Folder: ${folder.title.trim() || 'untitled'}`}>
      <header className="fold-detail-head">
        <span aria-hidden="true" className="text-[22px] leading-none">
          {folder.coverEmoji || '📁'}
        </span>
        <div className="min-w-0 flex-1">
          <h3 className={`m-0 truncate text-[17px] leading-[24px] font-medium ${folder.title.trim() ? '' : 'text-dim'}`}>
            {folder.title.trim() || 'Untitled folder'}
          </h3>
          <p className="type-data text-dim m-0 text-[12px]">
            {ordinal(position + 1)} of {total} · {appearanceSummary(folder)}
          </p>
        </div>
        <div className="fold-detail-actions">
          <RowIconButton
            icon={ChevronLeft}
            label={`Move ${label} left${position === 0 ? ', already first' : ''}`}
            disabled={position === 0}
            onClick={() => onMove(-1)}
          />
          <RowIconButton
            icon={ChevronRight}
            label={`Move ${label} right${position === total - 1 ? ', already last' : ''}`}
            disabled={position === total - 1}
            onClick={() => onMove(1)}
          />
          <button type="button" onClick={onRemove} className="btn-danger-text ml-3 h-8 text-[13px]">
            Remove
          </button>
        </div>
      </header>

      <div className="setting">
        <label htmlFor={`${idBase}-title`} className="setting-label type-label">
          Folder title
        </label>
        <div className="setting-value">
          <TextInput
            id={`${idBase}-title`}
            value={folder.title}
            onChange={(title) => onChange({ title })}
            placeholder="Folder title"
            ariaLabel={`Title of folder ${position + 1}`}
            invalid={Boolean(errors?.title)}
          />
          {errors?.title && <FieldError>{errors.title}</FieldError>}
        </div>
      </div>

      <FolderCatalogs folder={folder} errors={errors} {...catalogs} />

      <button
        type="button"
        className="sec-head fold-appearance"
        aria-expanded={appearanceOpen}
        onClick={() => setAppearanceOpen((current) => !current)}
      >
        <span className="setting-label type-label">Appearance</span>
        <span className="sec-sum text-dim text-[14px]">{appearanceSummary(folder)}</span>
        <Icon icon={ChevronDown} size={16} className="ico" />
      </button>

      {appearanceOpen && (
        <>
          <div className="setting">
            <span className="setting-label type-label">Hide the title</span>
            <div className="setting-value ed-line">
              <Segmented
                ariaLabel={`Hide the title above ${label}'s tiles`}
                value={folder.hideTitle ? 'hide' : 'show'}
                onChange={(value) => onChange({ hideTitle: value === 'hide' })}
                options={[
                  { value: 'show', label: 'Show it' },
                  { value: 'hide', label: 'Hide it' },
                ]}
              />
              <span className="ed-note">The tab itself still shows the name.</span>
            </div>
          </div>

          <div className="setting">
            <span className="setting-label type-label">Tile shape</span>
            <div className="setting-value choices" role="group" aria-label={`Tile shape for ${label}`}>
              {TILE_SHAPES.map((shape) => (
                <button
                  key={shape}
                  type="button"
                  className="choice"
                  aria-pressed={folder.tileShape === shape}
                  onClick={() => onChange({ tileShape: shape })}
                >
                  {shape.charAt(0) + shape.slice(1).toLowerCase()}
                </button>
              ))}
            </div>
          </div>

          <div className="setting">
            <label htmlFor={`${idBase}-emoji`} className="setting-label type-label">
              Cover
            </label>
            <div className="setting-value flex max-w-[360px] gap-2">
              <div className="w-[var(--w-code)] shrink-0">
                <TextInput
                  id={`${idBase}-emoji`}
                  value={folder.coverEmoji}
                  onChange={(coverEmoji) => onChange({ coverEmoji })}
                  placeholder="🎬"
                  maxLength={8}
                  ariaLabel={`Cover emoji for ${label}`}
                />
              </div>
              <div className="min-w-0 flex-1">
                <TextInput
                  value={folder.coverImageURL}
                  onChange={(coverImageURL) => onChange({ coverImageURL })}
                  placeholder="Image URL, https://…"
                  ariaLabel={`Cover image URL for ${label}`}
                />
              </div>
            </div>
          </div>

          <div className="setting">
            <label htmlFor={`${idBase}-gif`} className="setting-label type-label">
              Focus GIF
            </label>
            <div className="setting-value flex max-w-[360px] flex-col gap-2">
              <TextInput
                id={`${idBase}-gif`}
                value={folder.focusGIFURL}
                onChange={(focusGIFURL) => onChange({ focusGIFURL })}
                placeholder="GIF URL, https://…"
                ariaLabel={`Focus GIF URL for ${label}`}
              />
              <div className="ed-line">
                <Segmented
                  ariaLabel={`Play the focus GIF on ${label}`}
                  value={folder.focusGIFEnabled ? 'on' : 'off'}
                  onChange={(value) => onChange({ focusGIFEnabled: value === 'on' })}
                  options={[
                    { value: 'off', label: 'Off' },
                    { value: 'on', label: 'On' },
                  ]}
                />
                <span className="ed-note">Plays over the tile while it's focused.</span>
              </div>
            </div>
          </div>

          <HeroURLRow
            id={`${idBase}-hero-backdrop`}
            role="Hero backdrop"
            value={folder.heroBackdropURL}
            onChange={(heroBackdropURL) => onChange({ heroBackdropURL })}
            placeholder="Image URL, https://…"
            ariaLabel={`Modern Home hero backdrop URL for ${label}`}
          />
          <HeroURLRow
            id={`${idBase}-hero-video`}
            role="Hero video"
            value={folder.heroVideoURL}
            onChange={(heroVideoURL) => onChange({ heroVideoURL })}
            placeholder="Video URL, https://…"
            ariaLabel={`Modern Home hero video URL for ${label}`}
          />
          <HeroURLRow
            id={`${idBase}-title-logo`}
            role="Title logo"
            value={folder.titleLogoURL}
            onChange={(titleLogoURL) => onChange({ titleLogoURL })}
            placeholder="Image URL, https://…"
            ariaLabel={`Modern Home title logo URL for ${label}`}
            note="These three are for Nuvio's Modern Home layout."
          />
        </>
      )}
    </section>
  )
}

/** One of the folder's Modern Home hero URLs — the same label-and-field row as
 *  the folder's title. */
function HeroURLRow({
  id,
  role,
  value,
  onChange,
  placeholder,
  ariaLabel,
  note,
}: {
  id: string
  role: string
  value: string
  onChange: (value: string) => void
  placeholder: string
  ariaLabel: string
  note?: string
}) {
  return (
    <div className="setting">
      <label htmlFor={id} className="setting-label type-label">
        {role}
      </label>
      <div className="setting-value flex max-w-[360px] flex-col gap-2">
        <TextInput id={id} value={value} onChange={onChange} placeholder={placeholder} ariaLabel={ariaLabel} />
        {note && <span className="ed-note">{note}</span>}
      </div>
    </div>
  )
}

interface FolderCatalogsProps {
  folder: FolderFormState
  errors: FolderErrors | undefined
  options: RefOption[]
  /** Merged: the library plus every scoped catalog this editor already knows
   *  about — see `CollectionEditor`'s `mergedOptionByID`. Scoped catalogs
   *  never appear in `options` (only listed ones are linkable), but they do
   *  need to render once referenced. */
  optionByID: ReadonlyMap<string, RefOption>
  /** How many folders across every owned collection reference a catalog —
   *  only meaningful for a listed catalog, so the row asks with its own id. */
  usedInFolders: (catalogID: string) => number
  onAddRef: (catalogID: string) => void
  /** "Copy" from the picker: adds a fresh scoped copy as a new ref, rather
   *  than linking the listed catalog picked. */
  onCopyRefIntoCollection: (catalogID: string) => void
  onRemoveRef: (refKey: string) => void
  /** `''` clears the ref's genre back to unfiltered. */
  onSetRefGenre: (refKey: string, genre: string) => void
  /** "Add another genre": a second ref to this ref's catalog, under `genre`. */
  onAddGenreRef: (refKey: string, genre: string) => void
  onMoveRef: (refKey: string, direction: -1 | 1) => void
  onEditRef: (catalogID: string) => void
  /** "Copy into this collection" offered on an already-linked listed
   *  catalog's own row: replaces this ref with a fresh scoped copy in place,
   *  so the folder's order doesn't change. */
  onCopyRef: (refKey: string, catalogID: string) => void
  onAddNewInCollection: () => void
}

/** The folder's catalog list: its head with Add/New, the picker, and the
 *  ordered refs. */
function FolderCatalogs({
  folder,
  errors,
  options,
  optionByID,
  usedInFolders,
  onAddRef,
  onCopyRefIntoCollection,
  onRemoveRef,
  onSetRefGenre,
  onAddGenreRef,
  onMoveRef,
  onEditRef,
  onCopyRef,
  onAddNewInCollection,
}: FolderCatalogsProps) {
  const [picking, setPicking] = useState(false)
  // The picker adds an unfiltered ref, so it hides a catalog that already has
  // one here — a second would repeat the (catalog, genre) pair. A catalog
  // whose refs here are all narrowed to a genre stays pickable. Memoised
  // because it's the picker's `useMemo` dependency — a fresh Set every render
  // would re-filter the whole catalog list on every keystroke in the folder.
  const unfilteredInFolder = useMemo(
    () => new Set(folder.refs.filter((ref) => ref.genre === '').map((ref) => ref.catalogID)),
    [folder.refs],
  )
  const refKeys = useMemo(() => folder.refs.map((ref) => ref.key), [folder.refs])

  return (
    <>
      <div className="setting is-head">
        <span className="setting-label type-label">
          Catalogs <span className="text-dimmer tabular-nums">{folder.refs.length}</span>
        </span>
        <div className="setting-value flex flex-wrap items-center justify-end gap-2">
          {!picking && (
            <>
              <button type="button" onClick={onAddNewInCollection} className="btn-ghost btn-sm">
                New catalog
              </button>
              <button type="button" onClick={() => setPicking(true)} className="btn-secondary btn-sm">
                Add catalogs
              </button>
            </>
          )}
        </div>
      </div>

      {picking && (
        <div className="flex flex-col gap-2 pt-3">
          <p className="ed-note m-0">
            <Icon icon={Plus} size={12} className="mb-px inline" /> links a catalog, so edits to it
            show up everywhere it's used.{' '}
            <Icon icon={Copy} size={12} className="mb-px inline" /> copies it into this collection
            only.
          </p>
          <CatalogRefPicker
            options={options}
            exclude={unfilteredInFolder}
            onAdd={onAddRef}
            onCopy={onCopyRefIntoCollection}
            onClose={() => setPicking(false)}
          />
        </div>
      )}

      {errors?.catalogIDs && <FieldError>{errors.catalogIDs}</FieldError>}

      {folder.refs.length === 0 ? (
        <div className="py-3">
          <p className="ed-note m-0">This folder needs at least one catalog to show anything on your TV.</p>
        </div>
      ) : (
        <SortableContext
          items={refKeys.map((key) => refDragID(folder.key, key))}
          strategy={verticalListSortingStrategy}
        >
          <ul className="m-0 flex list-none flex-col p-0">
            {folder.refs.map((ref, index) => (
              <RefRow
                key={ref.key}
                folderKey={folder.key}
                refState={ref}
                refKeys={refKeys}
                siblingGenres={folder.refs
                  .filter((other) => other.key !== ref.key && other.catalogID === ref.catalogID)
                  .map((other) => other.genre)}
                position={index}
                total={folder.refs.length}
                option={optionByID.get(ref.catalogID)}
                usedInFolders={usedInFolders}
                onGenreChange={(genre) => onSetRefGenre(ref.key, genre)}
                onAddGenre={(genre) => onAddGenreRef(ref.key, genre)}
                onRemove={() => onRemoveRef(ref.key)}
                onMove={(direction) => onMoveRef(ref.key, direction)}
                onEdit={() => onEditRef(ref.catalogID)}
                onCopyIntoCollection={() => onCopyRef(ref.key, ref.catalogID)}
              />
            ))}
          </ul>
        </SortableContext>
      )}
    </>
  )
}
