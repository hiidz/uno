import { useId, useMemo, useState } from 'react'
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { ChevronDown, Plus } from 'lucide-react'
import type { TileShape } from '@/api'
import { FieldError, InfoTip, OnlyIn, Segmented, TextInput } from '@/components/fields'
import { Icon } from '@/components/Icon'
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

/** The folder's Appearance shelf, folded. */
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
 * The selected folder: a heading with where it sits in the row, then its title, its catalogs, and its appearance settings folded behind one
 * summarised row — the catalogs are what a folder is opened for, so they come
 * before the settings that are rarely touched.
 */
export function FolderDetail({
  folder,
  position,
  errors,
  onChange,
  onRemove,
  ...catalogs
}: {
  folder: FolderFormState
  position: number
  /** Undefined until the user has tried to save — same rule as the catalog
   *  builder, which withholds errors until submit. */
  errors: FolderErrors | undefined
  onChange: (update: Partial<FolderFormState>) => void
  onRemove: () => void
} & Omit<FolderCatalogsProps, 'folder' | 'errors'>) {
  const idBase = useId()
  const [appearanceOpen, setAppearanceOpen] = useState(false)
  const label = folderLabel(folder, position)

  return (
    <section className="fold-detail" aria-label={`Folder: ${folder.title.trim() || 'untitled'}`}>
      <header className="fold-detail-head">
        <h3 className="type-data m-0 min-w-0 flex-1 text-[15px] leading-[24px] font-semibold">
          {folder.title.trim() || 'Untitled folder'}
        </h3>
        <div className="fold-detail-actions">
          <button type="button" onClick={onRemove} className="btn-danger-text h-8 text-[13px]">
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
        <span className="setting-label type-label">Folder Appearance</span>
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
                <InfoTip
                  label="Focus GIF"
                  text="Nuvio TV plays it over the tile while it's selected. Nuvio's phone and desktop apps ignore this switch and show the GIF as the tile itself, unless it's turned off in that device's settings."
                />
              </div>
              <TextInput
                id={`${idBase}-gif`}
                value={folder.focusGIFURL}
                onChange={(focusGIFURL) => onChange({ focusGIFURL })}
                placeholder="GIF URL, https://…"
                ariaLabel={`Focus GIF URL for ${label}`}
              />
            </div>
          </div>

          <HeroURLRow
            id={`${idBase}-hero-backdrop`}
            role="Hero backdrop"
            value={folder.heroBackdropURL}
            onChange={(heroBackdropURL) => onChange({ heroBackdropURL })}
            placeholder="Image URL, https://…"
            ariaLabel={`Modern Home hero backdrop URL for ${label}`}
            note="Hero backdrop, hero video and title logo are used only by Nuvio TV's Modern Home layout."
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
      <div className="flex items-center gap-1.5">
        <label htmlFor={id} className="setting-label type-label text-ink">
          {role}
        </label>
        <OnlyIn where="Nuvio TV, Modern layout" />
        {note && <InfoTip label="Modern Home" text={note} />}
      </div>
      <div className="setting-value flex max-w-[360px] flex-col gap-2">
        <TextInput id={id} value={value} onChange={onChange} placeholder={placeholder} ariaLabel={ariaLabel} />
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
  onRemoveRef: (refKey: string) => void
  /** `''` clears the ref's genre back to unfiltered. */
  onSetRefGenre: (refKey: string, genre: string) => void
  /** "Add another genre": a second ref to this ref's catalog, under `genre`. */
  onAddGenreRef: (refKey: string, genre: string) => void
  onMoveRef: (refKey: string, direction: -1 | 1) => void
  onEditRef: (catalogID: string) => void
  /** `name` pre-fills the naming dialog — the search that found no catalog. */
  onAddNewInCollection: (name?: string) => void
}

/** The folder's catalog list: its head with New and Add, and the ordered
 *  refs. */
function FolderCatalogs({
  folder,
  errors,
  options,
  optionByID,
  usedInFolders,
  onAddRef,
  onRemoveRef,
  onSetRefGenre,
  onAddGenreRef,
  onMoveRef,
  onEditRef,
  onAddNewInCollection,
}: FolderCatalogsProps) {
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
          <CatalogRefPicker
            options={options}
            exclude={unfilteredInFolder}
            onAdd={(catalogIDs) => catalogIDs.forEach(onAddRef)}
            onNew={onAddNewInCollection}
          />
          <button type="button" onClick={() => onAddNewInCollection()} className="btn-secondary btn-sm">
            <Icon icon={Plus} size={13} />
            New catalog
          </button>
        </div>
      </div>

      {errors?.catalogIDs && <FieldError>{errors.catalogIDs}</FieldError>}

      {folder.refs.length === 0 ? (
        <div className="py-3">
          <p className="ed-note m-0">Empty folder — add a catalog.</p>
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
              />
            ))}
          </ul>
        </SortableContext>
      )}
    </>
  )
}
