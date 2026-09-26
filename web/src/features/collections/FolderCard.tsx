import { useId, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import {
  DndContext,
  closestCenter,
  type Announcements,
  type CollisionDetection,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  rectSortingStrategy,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { useQuery } from '@tanstack/react-query'
import {
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Copy,
  Plus,
  TriangleAlert,
} from 'lucide-react'
import { Grip, RowIconButton, useDragSensors } from '@/components/dnd'
import { fetchCatalogGenreOptions, queryKeys, type Catalog, type TileShape } from '@/api'
import { Segmented, Select, TextInput } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { MoreMenu, MoreMenuItem, MoreMenuSeparator } from '@/components/MoreMenu'
import { useHomeSelection } from '@/features/home/useHomeSelection'
import { TILE_ASPECT } from '@/features/preview/tiles'
import { ordinal } from '@/lib/ordinal'
import { reorder } from '@/lib/order'
import { pluralCount } from '@/lib/plural'
import { CatalogRefPicker } from './CatalogRefPicker'
import {
  TILE_SHAPES,
  type FolderErrors,
  type FolderFormState,
  type FolderRefState,
} from './collectionForm'
import type { RefOption } from './refs'

/**
 * The folder tree's drag behaviour, the folder tile strip, and the selected
 * folder's detail — DESIGN.md's "Folder strip".
 *
 * **One `DndContext`, several `SortableContext`s.** Folder tiles reorder among
 * themselves; each folder's catalog refs reorder among themselves. Nesting a
 * second `DndContext` inside the first does not isolate them — the outer
 * context still sees the inner drags. So every sortable declares which list it
 * belongs to via dnd-kit's `data`, and `onDragEnd` reorders only when the
 * dragged item and the one it was dropped on agree. A ref dragged out of its
 * folder is a no-op rather than a mis-drop.
 *
 * Moving a ref *between* folders is not supported: remove and re-add instead.
 */

const FOLDERS = '__folders__'

interface SortableData {
  container: string
}

function containerOf(node: { data: { current?: unknown } } | null | undefined): string | null {
  const data = node?.data.current as SortableData | undefined
  return data?.container ?? null
}

/**
 * Rank only the droppables in the dragged item's own list.
 *
 * Load-bearing. Folders and every ref row share one `DndContext`, and plain
 * `closestCenter` ranks all of them together — so a folder tile dragged down
 * toward the open folder's catalogs finds a *ref row* to be the nearest center.
 * `onDragEnd` then sees two different containers, refuses to guess, and the
 * folder springs back. Narrowing the candidate set here leaves
 * that container check as a backstop rather than the mechanism.
 */
const withinContainer: CollisionDetection = (args) => {
  const container = containerOf(args.active)
  return closestCenter({
    ...args,
    droppableContainers: args.droppableContainers.filter(
      (candidate) => containerOf(candidate) === container,
    ),
  })
}

/** A ref's drag id, from its form `key` rather than its catalog id: one
 *  catalog can be two refs in a folder, under two genres. Prefixed with the
 *  folder key so `onDragEnd` can recover which list it belongs to. */
function refDragID(folderKey: string, refKey: string): string {
  return `${folderKey}::${refKey}`
}

/**
 * dnd-kit's default live-region announcements read out the raw drag id
 * ("Draggable item f1 was moved over droppable area f2") — meaningless to a
 * screen reader user, since ids are session-local keys or catalog uuids, not
 * anything shown on screen. This translates every id back to the name
 * already visible for it before handing it to the drag's `aria-live` region.
 */
function buildAnnouncements(
  folderName: (folderKey: string) => string,
  refName: (refKey: string) => string,
): Announcements {
  function describe(id: string | number): string {
    const raw = String(id)
    const separator = raw.indexOf('::')
    if (separator === -1) return `folder “${folderName(raw)}”`
    return `catalog “${refName(raw.slice(separator + 2))}”`
  }
  return {
    onDragStart: ({ active }) => `Picked up ${describe(active.id)}.`,
    onDragOver: ({ active, over }) =>
      over
        ? `${describe(active.id)} is now over ${describe(over.id)}.`
        : `${describe(active.id)} is no longer over a droppable area.`,
    onDragEnd: ({ active, over }) =>
      over
        ? `${describe(active.id)} was dropped over ${describe(over.id)}.`
        : `${describe(active.id)} was dropped.`,
    onDragCancel: ({ active }) => `Reordering ${describe(active.id)} was cancelled.`,
  }
}

export function FolderTreeDnd({
  folderKeys,
  folderName,
  refName,
  onReorderFolders,
  onReorderRefs,
  children,
}: {
  folderKeys: string[]
  /** Looks up a folder's display name from its `key` — used only to word the
   *  drag-and-drop live-region announcements. */
  folderName: (folderKey: string) => string
  /** Looks up a catalog ref's display name from its `key` — same. */
  refName: (refKey: string) => string
  onReorderFolders: (orderedKeys: string[]) => void
  onReorderRefs: (folderKey: string, orderedRefKeys: string[]) => void
  children: ReactNode
}) {
  const sensors = useDragSensors()
  const announcements = useMemo(
    () => buildAnnouncements(folderName, refName),
    [folderName, refName],
  )

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event
    if (!over || active.id === over.id) return

    const container = containerOf(active)
    // A drop onto a different list is ignored rather than guessed at.
    if (!container || container !== containerOf(over)) return

    const activeID = String(active.id)
    const overID = String(over.id)

    if (container === FOLDERS) {
      onReorderFolders(reorder(folderKeys, activeID, overID))
      return
    }

    const ids = (active.data.current as (SortableData & { ids: string[] }) | undefined)?.ids
    if (!ids) return
    const ordered = reorder(
      ids.map((id) => refDragID(container, id)),
      activeID,
      overID,
    ).map((dragID) => dragID.slice(container.length + 2))
    onReorderRefs(container, ordered)
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={withinContainer}
      onDragEnd={handleDragEnd}
      accessibility={{ announcements }}
    >
      <SortableContext items={folderKeys} strategy={rectSortingStrategy}>
        {children}
      </SortableContext>
    </DndContext>
  )
}

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

export function folderLabel(folder: FolderFormState, position: number): string {
  return folder.title.trim() || `folder ${position + 1}`
}

/**
 * The folder strip: each folder drawn at its own `tile_shape`, the way the TV
 * draws the collection's row. One is always selected, and its contents open
 * beneath the strip in `FolderDetail`.
 */
export function FolderTiles({
  folders,
  selectedKey,
  errorKeys,
  onSelect,
}: {
  folders: FolderFormState[]
  selectedKey: string | null
  /** Folders with a validation error, marked on their tile so a problem in a
   *  folder that isn't open is still visible. */
  errorKeys: ReadonlySet<string>
  onSelect: (key: string) => void
}) {
  return (
    <ul className="fold-tiles">
      {folders.map((folder, index) => (
        <FolderTileItem
          key={folder.key}
          folder={folder}
          position={index}
          selected={folder.key === selectedKey}
          hasError={errorKeys.has(folder.key)}
          onSelect={() => onSelect(folder.key)}
        />
      ))}
    </ul>
  )
}

const TILE_H = 104

function FolderTileItem({
  folder,
  position,
  selected,
  hasError,
  onSelect,
}: {
  folder: FolderFormState
  position: number
  selected: boolean
  hasError: boolean
  onSelect: () => void
}) {
  const sortable = useSortable({
    id: folder.key,
    data: { container: FOLDERS } satisfies SortableData,
  })
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = sortable
  const width = TILE_H * TILE_ASPECT[folder.tileShape]
  const name = folder.title.trim() || 'Untitled folder'

  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition, width }}
      className={`fold-tile ${selected ? 'is-selected' : ''} ${isDragging ? 'relative z-10 opacity-40' : ''}`}
    >
      <button
        type="button"
        onClick={onSelect}
        aria-pressed={selected}
        aria-label={`${name}, ${pluralCount(folder.refs.length, 'catalog')}${hasError ? ', needs fixing' : ''}`}
        className="fold-tile-btn"
      >
        <span className="fold-tile-face" style={{ height: TILE_H }}>
          {folder.coverEmoji ? (
            <span aria-hidden="true" className="text-[26px] leading-none">
              {folder.coverEmoji}
            </span>
          ) : (
            <span aria-hidden="true" className="text-dimmer line-clamp-2 px-2 text-center text-[12.5px] leading-tight">
              {name}
            </span>
          )}
          {/* Layered over the emoji/title rather than replacing them, so a
              cover URL that 404s or is slow leaves a legible tile. */}
          {folder.coverImageURL.trim() && (
            <img src={folder.coverImageURL} alt="" loading="lazy" className="absolute inset-0 h-full w-full object-cover" />
          )}
        </span>
        <span className="fold-tile-name">
          {hasError && <Icon icon={TriangleAlert} size={12} className="text-danger shrink-0" />}
          <span className="truncate">{name}</span>
        </span>
        <span className="type-data text-dimmer text-[12.5px] tabular-nums">
          {pluralCount(folder.refs.length, 'catalog')}
        </span>
      </button>
      <span className="fold-tile-grip">
        <Grip label={`Reorder ${folderLabel(folder, position)}`} sortable={{ attributes, listeners }} />
      </span>
    </li>
  )
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
          {errors?.title && <FolderError>{errors.title}</FolderError>}
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

      {errors?.catalogIDs && <FolderError>{errors.catalogIDs}</FolderError>}

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

/**
 * One catalog reference. Order is the value here — index becomes
 * `folder_catalogs.sort_order` — so the position is stated as well as draggable.
 *
 * An option this profile can't resolve is the *same* condition as a save that
 * would 400: `optionByID` merges the library with every scoped catalog this
 * editor already knows about (`CollectionEditor`'s `mergedOptionByID`), which
 * is exactly the closed-graph folder-ref rule `validateFolderRefs` checks
 * server-side.
 *
 * **Quiet Edit is scoped-catalog only.** It opens the referenced catalog one
 * level down, in a modal over this editor — `CollectionEditor`'s own nested
 * `CatalogEditor`, not the main pane — and a scoped catalog is only ever
 * used here, so editing it in place is unambiguous. A *listed* one is a live
 * pointer the same as it always was — editing it here would silently reach
 * every other folder and the library too — so this row has no inline edit
 * for it at all: it states how many places it's used and offers "Copy into
 * this collection" for whoever wants an independent, editable copy instead.
 * Editing a listed catalog directly is the library rail's job.
 */
function RefRow({
  folderKey,
  refState,
  refKeys,
  siblingGenres,
  position,
  total,
  option,
  usedInFolders,
  onGenreChange,
  onAddGenre,
  onRemove,
  onMove,
  onEdit,
  onCopyIntoCollection,
}: {
  folderKey: string
  refState: FolderRefState
  /** Carried in the drag payload so `onDragEnd` can reorder this folder's list
   *  without reaching back into the form state. */
  refKeys: string[]
  /** The genres this folder's other refs to the same catalog already use —
   *  the pairs this ref may not take, and "Add another genre" may not add. */
  siblingGenres: string[]
  position: number
  total: number
  option: RefOption | undefined
  usedInFolders: (catalogID: string) => number
  onGenreChange: (genre: string) => void
  onAddGenre: (genre: string) => void
  onRemove: () => void
  onMove: (direction: -1 | 1) => void
  onEdit: () => void
  /** Offered on a listed catalog's row only; see `RefMenu`. */
  onCopyIntoCollection: () => void
}) {
  const sortable = useSortable({
    id: refDragID(folderKey, refState.key),
    data: { container: folderKey, ids: refKeys },
  })
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = sortable

  const genreOptions = useGenreOptions(option?.catalog)
  const taken = new Set([refState.genre, ...siblingGenres])
  const nextGenre = genreOptions.data?.find((g) => !taken.has(g.name))?.name

  const isScoped = option ? option.catalog.collection_id !== null : false
  // The home screen is counted here, not in `usedInFolders`, so a change to
  // the selection re-renders the rows that state it and not the workspace.
  const home = useHomeSelection()
  const places =
    option && !isScoped
      ? usedInFolders(refState.catalogID) + (home.hasCatalog(refState.catalogID) ? 1 : 0)
      : 0

  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`run-row ${isDragging ? 'bg-raised relative z-10 opacity-40' : ''}`}
    >
      <div className="run-ctl">
        <Grip label={`Reorder ${option?.name ?? 'unavailable catalog'}`} sortable={{ attributes, listeners }} />
        <span className="type-data text-dimmer w-8 shrink-0 text-right text-[12.5px] tabular-nums">
          {position + 1}
        </span>
      </div>

      <div className="flex min-w-0 items-center gap-3">
        {!option && (
          <span aria-hidden="true" className="text-danger bg-current w-[3px] shrink-0 self-stretch rounded-[1px]" />
        )}

        {option ? (
          <span className="flex min-w-0 flex-1 flex-col gap-0.5">
            <span className="truncate text-[13px] font-medium">{option.name}</span>
            <span className="type-data text-dimmer text-[12.5px] leading-[1.45]">
              {option.recipe} · {option.catalog.type}
              {!isScoped && ` · used in ${pluralCount(places, 'place')}`}
            </span>
            <RefGenrePicker
              query={genreOptions}
              name={option.name}
              genre={refState.genre}
              siblingGenres={siblingGenres}
              onChange={onGenreChange}
            />
          </span>
        ) : (
          <span className="flex min-w-0 flex-1 flex-col gap-0.5">
            <span className="text-danger truncate text-[13px]">Unavailable catalog</span>
            <span className="type-data text-dimmer truncate text-[12.5px]">Deleted</span>
          </span>
        )}

        <div className="flex shrink-0 items-center gap-1">
          {isScoped ? (
            <button type="button" onClick={onEdit} className="btn-ghost px-2 text-[12px]">
              Edit
            </button>
          ) : !option ? (
            <button type="button" onClick={onRemove} className="btn-ghost px-2 text-[12px]">
              Remove
            </button>
          ) : null}
          <RefMenu
            label={option?.name ?? 'this catalog'}
            first={position === 0}
            last={position === total - 1}
            onMove={onMove}
            onRemove={onRemove}
            nextGenre={nextGenre}
            onAddGenre={option ? onAddGenre : undefined}
            onCopyIntoCollection={option && !isScoped ? onCopyIntoCollection : undefined}
          />
        </div>
      </div>
    </li>
  )
}

/**
 * The genres a pick can narrow `catalog`'s recipe by
 * (`POST /api/catalogs/genre-options`) — the list the manifest advertises and
 * the addon path resolves against. Idle for an unresolvable ref, which has no
 * recipe to ask about.
 */
function useGenreOptions(catalog: Pick<Catalog, 'type' | 'params'> | undefined) {
  return useQuery({
    queryKey: queryKeys.catalogGenreOptions(catalog?.type ?? 'movie', catalog?.params ?? ''),
    queryFn: () => fetchCatalogGenreOptions({ type: catalog!.type, params: catalog!.params }),
    enabled: catalog !== undefined,
    staleTime: 5 * 60_000,
    retry: false,
  })
}

/**
 * Narrows one folder reference to a genre — pushed as its source's `genre`,
 * which Nuvio sends back as the catalog's genre extra when it loads the row.
 *
 * Offers the recipe's own genre options, so every choice here actually narrows
 * the row, minus the genres this folder's other refs to the same catalog
 * already use: the same catalog under the same genre twice is a repeat the
 * primary key forbids. A stored genre that's no longer among the options — the
 * recipe has since been edited to require or exclude it — is kept and flagged
 * rather than cleared: the addon path serves that row unfiltered, and the user
 * decides what to do about it.
 */
function RefGenrePicker({
  query,
  name,
  genre,
  siblingGenres,
  onChange,
}: {
  query: ReturnType<typeof useGenreOptions>
  name: string
  genre: string
  siblingGenres: string[]
  onChange: (genre: string) => void
}) {
  const offered = query.data ?? []
  const stale = genre !== '' && query.isSuccess && !offered.some((g) => g.name === genre)
  const free = (value: string) => value === genre || !siblingGenres.includes(value)
  const options = [
    ...(free('') ? [{ value: '', label: 'All genres' }] : []),
    // "Only …" rather than the bare name, so the closed control says what it
    // does without a label of its own.
    ...offered.filter((g) => free(g.name)).map((g) => ({ value: g.name, label: `Only ${g.name}` })),
    // Until the list lands (or if it can't), the stored value still needs an
    // option to show as selected.
    ...(genre !== '' && !offered.some((g) => g.name === genre)
      ? [{ value: genre, label: stale ? `Only ${genre} (no longer applies)` : `Only ${genre}` }]
      : []),
  ]

  return (
    <span className="mt-1.5 flex flex-col gap-1">
      <Select
        value={genre}
        onChange={onChange}
        options={options}
        width="var(--w-pick)"
        ariaLabel={`Genre shown from ${name}`}
      />
      {stale && (
        <span className="type-data text-danger text-[12.5px] leading-[1.45]">
          This catalog's filters no longer allow {genre}, so your TV shows it unfiltered.
        </span>
      )}
      {query.isError && (
        <span className="type-data text-dimmer text-[12.5px] leading-[1.45]">
          Couldn't load this catalog's genres.
        </span>
      )}
    </span>
  )
}

/** DESIGN.md's field error line — icon column, then the message — the same
 *  markup the collection's own title uses. */
function FolderError({ children }: { children: ReactNode }) {
  return (
    <p className="field-error">
      <Icon icon={TriangleAlert} size={16} className="text-danger" />
      <span>{children}</span>
    </p>
  )
}

/**
 * Everything on a catalog row besides Edit: Edit is the row's one inline
 * action, and dragging the grip (pointer, touch or keyboard) is the main way
 * to reorder, so "Add another genre", Move up/down, Remove and "Copy into this
 * collection" wait behind "⋯" and the name keeps the row's width at phone size.
 */
function RefMenu({
  label,
  first,
  last,
  onMove,
  onRemove,
  nextGenre,
  onAddGenre,
  onCopyIntoCollection,
}: {
  label: string
  first: boolean
  last: boolean
  onMove: (direction: -1 | 1) => void
  onRemove: () => void
  /** The next genre this ref's catalog can be narrowed by. Absent before the
   *  genre options land, and once every one of them is taken by a ref to this
   *  catalog in this folder. */
  nextGenre?: string
  /** Absent for an unavailable catalog, which has no genres to offer. */
  onAddGenre?: (genre: string) => void
  /** Absent for a scoped or unavailable catalog — nothing else can reference
   *  it, so there's nothing to copy it away from. */
  onCopyIntoCollection?: () => void
}) {
  return (
    <MoreMenu label={label}>
      {onAddGenre && (
        <MoreMenuItem
          disabled={nextGenre === undefined}
          onSelect={() => {
            if (nextGenre !== undefined) onAddGenre(nextGenre)
          }}
        >
          Add another genre
        </MoreMenuItem>
      )}
      {onCopyIntoCollection && (
        <MoreMenuItem onSelect={onCopyIntoCollection}>Copy into this collection</MoreMenuItem>
      )}
      {(onAddGenre || onCopyIntoCollection) && <MoreMenuSeparator />}
      <MoreMenuItem disabled={first} onSelect={() => onMove(-1)}>
        Move up
      </MoreMenuItem>
      <MoreMenuItem disabled={last} onSelect={() => onMove(1)}>
        Move down
      </MoreMenuItem>
      <MoreMenuSeparator />
      <MoreMenuItem onSelect={onRemove}>Remove from folder</MoreMenuItem>
    </MoreMenu>
  )
}
