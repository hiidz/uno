import { useEffect, useMemo, useState } from 'react'
import { tmdbKind } from '@/api'
import { TypeBar } from '@/components/TypeBar'
import { CollectionMeta } from '@/features/preview/CollectionMeta'
import { FolderPage } from '@/features/preview/FolderPage'
import type { PreviewChrome } from '@/features/preview/FolderPage'
import type { PreviewCollection, PreviewFolder } from '@/features/preview/model'
import { CONTENT_TILE_SHAPE, FolderTile, Note, TileStrip, TilesNote, noTiles } from '@/features/preview/tiles'
import type { CatalogTiles } from '@/features/preview/tiles'
import { useHomeSelection } from './useHomeSelection'
import { useCatalogTiles } from './useCatalogTiles'
import { buildHomePreview, findFolderPage } from './preview'
import type { FolderPageTarget, HomeScreenPreview, PreviewRow } from './preview'

/**
 * The Home pane's second view: the same pending state, drawn as the shape of
 * the home screen it will become. Reorder in List, flip to here, see it move.
 *
 * **Two levels, matching the real screen.** Home is one page — pinned
 * collections, then catalog rows, then the rest of the collections. A
 * collection is a single row whose tiles are its *folders*. Clicking a folder
 * tile opens that folder's page, where the collection's `view_mode` decides
 * whether its catalogs are tabs over a grid or stacked rows.
 *
 * **Read-only by construction.** The one interaction — opening a folder — is
 * navigation within the mock, not an edit. Every edit lives in the List view.
 */
export function HomePreview() {
  const home = useHomeSelection()

  const preview = useMemo(
    () =>
      buildHomePreview({
        catalogs: home.catalogs,
        collections: home.collections,
        catalogById: home.catalogById,
        collectionById: home.collectionById,
      }),
    [home.catalogs, home.collections, home.catalogById, home.collectionById],
  )

  // Which folder page is open; `null` is home itself. Held as ids, not indices,
  // so removing the collection or folder in the List view falls back to home
  // rather than pointing at whatever now sits in the same slot —
  // `findFolderPage` returns `null` for a target that no longer resolves.
  const [target, setTarget] = useState<FolderPageTarget | null>(null)
  const page = target ? findFolderPage(preview, target) : null

  // Clear the target too, not just the rendered page: an unresolvable target
  // left in state reopens the page if the same id comes back (remove a
  // collection in List, add it again).
  useEffect(() => {
    if (target !== null && page === null) setTarget(null)
  }, [target, page])

  if (preview.isEmpty) return <EmptyHomeScreen />

  return (
    <div className="border-line bg-raised flex flex-col overflow-hidden rounded-[2px] border">
      <div className="bg-ground min-h-[280px] p-5">
        {page ? (
          <HomeFolderPage
            collection={page.collection}
            folder={page.folder}
            onBack={() => setTarget(null)}
          />
        ) : (
          <HomeScreen preview={preview} onOpenFolder={setTarget} />
        )}
      </div>
    </div>
  )
}

/**
 * The shared folder page, fetched and annotated the way Home needs it.
 *
 * Every catalog in the folder is fetched on open: both layouts need the same
 * set, so switching tabs inside never triggers a new call.
 */
function HomeFolderPage({
  collection,
  folder,
  onBack,
}: {
  collection: PreviewCollection
  folder: PreviewFolder
  onBack: () => void
}) {
  const tiles = useCatalogTiles(folder.sources.map((source) => source.id))
  const chrome = useHomeChrome()
  return (
    <FolderPage
      collection={collection}
      folder={folder}
      tiles={tiles}
      chrome={chrome}
      onBack={onBack}
    />
  )
}

/** Home can say something about a catalog the builder can't: that a row still
 *  on the home screen has since left the library. */
function useHomeChrome(): PreviewChrome {
  const home = useHomeSelection()
  return {
    isOwned: home.isOwned,
    note: (id) => <DetachedNote id={id} />,
  }
}

/**
 * Home, in the order the screen renders it: pinned collection rows above
 * everything, then the catalog rows, then the remaining collection rows.
 *
 * Discover-only catalogs come last and outside the three bands, because they
 * are precisely the thing that is *not* on this screen.
 */
function HomeScreen({
  preview,
  onOpenFolder,
}: {
  preview: HomeScreenPreview
  onOpenFolder: (target: FolderPageTarget) => void
}) {
  const nothingOnHome =
    preview.rows.length === 0 &&
    preview.pinnedCollections.length === 0 &&
    preview.unpinnedCollections.length === 0

  // Every row's tiles fetched together when Preview opens: a row is one TMDB
  // page and that page is the whole row, so there is never a second call to
  // defer.
  const tiles = useCatalogTiles(preview.rows.map((row) => row.id))

  return (
    <div className="flex flex-col gap-7">
      {preview.pinnedCollections.map((collection) => (
        <CollectionRow key={collection.id} collection={collection} onOpenFolder={onOpenFolder} />
      ))}

      {preview.rows.map((row) => (
        <CatalogRow key={row.id} row={row} tiles={tiles.get(row.id) ?? noTiles()} />
      ))}

      {preview.unpinnedCollections.map((collection) => (
        <CollectionRow key={collection.id} collection={collection} onOpenFolder={onOpenFolder} />
      ))}

      {nothingOnHome && (
        <p className="type-data text-dimmer m-0 text-[11px]">
          Nothing on home — every selected catalog is set to Discover only.
        </p>
      )}

      {preview.discoverOnly.length > 0 && <DiscoverOnly rows={preview.discoverOnly} />}
    </div>
  )
}

/* -------------------------------------------------------------------------- */
/* Home — collection rows                                                     */
/* -------------------------------------------------------------------------- */

/**
 * A collection is **one row on home**, and its tiles are its folders — not its
 * content. Nothing here draws a catalog: a folder's tile is its cover emoji,
 * its title, and its own `tile_shape`.
 *
 * That last one is a folder field describing the folder's *own* tile, so
 * folders in one collection can disagree and the row can be ragged. Drawn
 * as-is — the raggedness is part of what is being previewed, not a glitch to
 * normalise away.
 */
function CollectionRow({
  collection,
  onOpenFolder,
}: {
  collection: PreviewCollection
  onOpenFolder: (target: FolderPageTarget) => void
}) {
  const home = useHomeSelection()

  return (
    <section className="flex flex-col gap-2">
      <div className="flex items-center gap-2.5">
        <TypeBar
          kind="collection"
          owned={home.isOwned(collection.id)}
          className="h-[13px] self-auto"
        />
        <h3 className="m-0 text-[13px] font-medium">{collection.title}</h3>
        {collection.pinned && <Note>pinned to the top of home</Note>}
        <DetachedNote id={collection.id} />
      </div>

      {/* Two distinct failures. `missing` means nothing in this browser can
          describe the collection at all — neither the library nor the selection
          response has it — so there is no layout to draw. Detached means the
          selection response still carries it (so it renders, and still works on
          the TV) but the library no longer lists it. */}
      {collection.missing ? (
        <p className="type-data text-dimmer m-0 text-[11px]">
          This collection is no longer available, so there's nothing to draw. It stays on your home
          screen until you remove it in the List view.
        </p>
      ) : collection.folders.length === 0 ? (
        <p className="type-data text-dimmer m-0 text-[11px]">
          This collection has no folders, so its row is empty.
        </p>
      ) : (
        // Scrolls rather than clips, matching the same row in the collection
        // builder. A folder past the pane's edge is a folder you can open on a
        // TV, so it has to be reachable here too — and narrow enough, the
        // second one was already gone.
        <div className="flex items-end gap-3 overflow-x-auto overscroll-x-contain">
          {collection.folders.map((folder) => (
            <FolderTile
              key={folder.id}
              folder={folder}
              onOpen={() => onOpenFolder({ collectionId: collection.id, folderId: folder.id })}
            />
          ))}
        </div>
      )}

      <CollectionMeta collection={collection} />
    </section>
  )
}

/* -------------------------------------------------------------------------- */
/* Home — catalog rows                                                        */
/* -------------------------------------------------------------------------- */

function CatalogRow({ row, tiles }: { row: PreviewRow; tiles: CatalogTiles }) {
  const home = useHomeSelection()

  return (
    <section className="flex flex-col gap-2">
      {/* `DetachedNote` and `TilesNote` can both fire on one row — a catalog
          whose owner made it private *and* whose fetch failed — so the header
          wraps rather than overflowing in a narrow pane. */}
      <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
        <TypeBar kind={row.type} owned={home.isOwned(row.id)} className="h-[13px] self-auto" />
        <h3 className="m-0 min-w-0 truncate text-[13px] font-medium">{row.name}</h3>
        <DetachedNote id={row.id} />
        <TilesNote tiles={tiles} />
      </div>
      <TileStrip shape={CONTENT_TILE_SHAPE} tiles={tiles} kind={tmdbKind(row.type)} />
    </section>
  )
}

/**
 * `show_in_home = false` means "stay in Discover, don't take a row on home".
 * The manifest builder does not consume the flag (`buildManifest` in
 * `internal/addon/addon.go`), so these rows still appear on the real home
 * screen. Dropping them silently here would imply a split that isn't live —
 * hence a labelled group carrying the same caveat as the List view's toggle.
 */
function DiscoverOnly({ rows }: { rows: PreviewRow[] }) {
  const home = useHomeSelection()

  return (
    <section className="border-line flex flex-col gap-2 border-t pt-4">
      <div className="flex items-baseline gap-3">
        <span className="type-eyebrow">Discover only</span>
        <span className="type-data text-dimmer text-[10px]">
          not active yet — these still show on your home screen
        </span>
      </div>
      <div className="flex flex-col gap-1">
        {rows.map((row) => (
          <div key={row.id} className="flex items-center gap-2.5 opacity-55">
            <TypeBar kind={row.type} owned={home.isOwned(row.id)} className="h-[11px] self-auto" />
            <span className="text-dim truncate text-[12px]">{row.name}</span>
            <DetachedNote id={row.id} />
          </div>
        ))}
      </div>
    </section>
  )
}

/**
 * "Not in library" is **not** the same question as "did this row resolve".
 * `catalogById` is assembled from the selection response *and* the library, so
 * a row whose owner has made it private since it was selected still resolves —
 * the selection endpoint joins through `profile_catalogs` with no visibility
 * filter. Hence `isDetached`, in the List view's wording: the two views must
 * not disagree about the same row.
 */
function DetachedNote({ id }: { id: string }) {
  const home = useHomeSelection()
  if (!home.isDetached(id)) return null

  return (
    <span
      className="type-data text-series shrink-0 text-[10px]"
      title="Its owner deleted it or made it private. It still works on your home screen, but removing it here can't be undone."
    >
      · not in library
    </span>
  )
}

/**
 * An empty home screen renders SMPTE colour bars — television's own artifact
 * for "nothing to show". The only place the palette goes to full amplitude.
 */
function EmptyHomeScreen() {
  const bars = [
    'var(--smpte-white)',
    'var(--smpte-yellow)',
    'var(--smpte-cyan)',
    'var(--smpte-green)',
    'var(--smpte-magenta)',
    'var(--smpte-red)',
    'var(--smpte-blue)',
  ]

  return (
    <div className="border-line relative overflow-hidden rounded-[2px] border">
      <div aria-hidden="true" className="flex h-[280px]">
        {bars.map((color) => (
          <span key={color} className="flex-1" style={{ background: color }} />
        ))}
      </div>
      <div className="absolute inset-x-0 top-1/2 -translate-y-1/2">
        <p className="type-display bg-ground text-ink m-0 px-5 py-3 text-center text-[13px]">
          No signal — nothing on your home screen yet
        </p>
      </div>
      <p className="type-data text-dimmer border-line m-0 border-t px-5 py-2 text-[10px]">
        Add catalogs and collections from the sidebar, then push.
      </p>
    </div>
  )
}
