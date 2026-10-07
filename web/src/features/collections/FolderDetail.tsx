import { useId, useMemo, useState } from 'react'
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { ChevronDown } from 'lucide-react'
import type { TileShape } from '@/api'
import { FieldError, InfoTip, OnlyIn, Segmented, TextInput } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { CatalogRefPicker } from './CatalogRefPicker'
import { CatalogRow } from './CatalogRow'
import {
  MAX_REFS_PER_FOLDER,
  TILE_SHAPES,
  folderLabel,
  refGroups,
  type FolderErrors,
  type FolderFormState,
  type FolderUnit,
} from './collectionForm'
import { dragID } from './folderDnd'
import { catalogOrder } from './folderEdits'
import type { RefOption } from './refs'
import type { CopyToLibrary } from './useCopyToLibrary'

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
  /** What one folder entry is in Nuvio, by the collection's view mode. */
  unit: FolderUnit
  /** How many folders across every owned collection reference a catalog —
   *  only meaningful for a listed catalog, so the row asks with its own id. */
  usedInFolders(catalogID: string): number
  /** An unfiltered ref to `catalogID`, at the end of the folder. */
  onAddRef(catalogID: string): void
  /** Every ref to `catalogID` out: the dropdown's untick and a row's
   *  Remove from folder. */
  onRemoveCatalog(catalogID: string): void
  /** A folder row's Copy into library. */
  onCopyToLibrary: CopyToLibrary
  /** A folder row's Unlink from library: every ref to `catalogID` moved to a
   *  catalog of its own, made from it, which only this collection has. */
  onUnlinkCatalog(catalogID: string): void
  onReorderCatalogs(catalogIDs: string[]): void
  onMoveCatalog(catalogID: string, direction: -1 | 1): void
  /** A genre ticked or a chip's removal under a catalog's line. */
  onAddGenre(catalogID: string, genre: string): void
  onRemoveRef(refKey: string): void
  onReorderGenres(catalogID: string, refKeys: string[]): void
  onEditRef(catalogID: string): void
  /** `name` pre-fills the naming dialog — the search that found no catalog. */
  onAddNewInCollection(name?: string): void
}

/** The folder's catalog list: its head with Add catalogs (which holds New),
 *  and one line per catalog, its genres under it. */
function FolderCatalogs({ folder, errors, options, optionByID, unit, ...actions }: FolderCatalogsProps) {
  // A catalog is ticked in the picker when the folder holds it under any
  // genre. Memoised because it's the picker's `useMemo` dependency — a fresh
  // Set every render would re-filter the whole catalog list on every
  // keystroke in the folder.
  const inFolder = useMemo(() => new Set(catalogOrder(folder.refs)), [folder.refs])
  const groups = useMemo(() => refGroups(folder.refs), [folder.refs])
  const catalogIDs = useMemo(() => catalogOrder(folder.refs), [folder.refs])

  return (
    <>
      <div className="setting is-head">
        <span className="setting-label type-label">
          Catalogs <span className="text-dimmer tabular-nums">{groups.length}</span>
        </span>
        <div className="setting-value flex flex-wrap items-center justify-end gap-2">
          <CatalogRefPicker
            options={options}
            inFolder={inFolder}
            full={folder.refs.length >= MAX_REFS_PER_FOLDER}
            onAdd={actions.onAddRef}
            onRemove={actions.onRemoveCatalog}
            onNew={actions.onAddNewInCollection}
          />
        </div>
      </div>

      {errors?.catalogIDs && <FieldError>{errors.catalogIDs}</FieldError>}

      {groups.length === 0 ? (
        <div className="py-3">
          <p className="ed-note m-0">Empty folder — add a catalog.</p>
        </div>
      ) : (
        <SortableContext
          items={catalogIDs.map((id) => dragID(folder.key, id))}
          strategy={verticalListSortingStrategy}
        >
          <ul className="m-0 flex list-none flex-col p-0">
            {groups.map((group, index) => (
              <CatalogRow
                key={group.catalogID}
                folderKey={folder.key}
                group={group}
                catalogIDs={catalogIDs}
                position={index}
                option={optionByID.get(group.catalogID)}
                unit={unit}
                usedInFolders={actions.usedInFolders}
                onReorder={actions.onReorderCatalogs}
                onAddGenre={(genre) => actions.onAddGenre(group.catalogID, genre)}
                onRemoveRef={actions.onRemoveRef}
                onReorderGenres={(refKeys) => actions.onReorderGenres(group.catalogID, refKeys)}
                onMove={(direction) => actions.onMoveCatalog(group.catalogID, direction)}
                onRemove={() => actions.onRemoveCatalog(group.catalogID)}
                onEdit={() => actions.onEditRef(group.catalogID)}
                onCopyToLibrary={actions.onCopyToLibrary}
                onUnlink={() => actions.onUnlinkCatalog(group.catalogID)}
              />
            ))}
          </ul>
        </SortableContext>
      )}
    </>
  )
}
