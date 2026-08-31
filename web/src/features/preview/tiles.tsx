import type { PreviewItem, TileShape, TMDBKind } from '@/api'
import type { PreviewFolder } from './model'

/**
 * Drawing a catalog's content: the tiles themselves, the shapes they're drawn
 * at, and the note that says what the tiles couldn't.
 *
 * Shared by the Home pane's Preview view and by both builder forms, which is
 * why it lives outside `features/home`. Everything here is presentational and
 * takes `CatalogTiles` — how those tiles were fetched (for a saved catalog, or
 * for a recipe still being typed) is the caller's problem.
 */

/** One TMDB discover page. Every view except the folder page's "All" tab is
 *  bounded by it, and the loading placeholders match the count so a row doesn't
 *  reflow when real posters land. */
export const TILES_PER_PAGE = 20

/** Tile proportion, width ÷ height, at each shape. Drawn at true proportions so
 *  `tile_shape` is judgeable at a glance rather than as an abstract enum. */
export const TILE_ASPECT: Record<TileShape, number> = {
  POSTER: 2 / 3,
  LANDSCAPE: 16 / 9,
  SQUARE: 1,
}

/**
 * The shape of a **content** tile — a title inside a catalog row or grid.
 *
 * `tile_shape` is a folder field and applies to the *folder's own tile* on
 * home, nothing else, so content tiles have no stored shape to read. Poster is
 * an assumption — the convention in every client in this space — and the
 * renderer says so on screen.
 */
export const CONTENT_TILE_SHAPE: TileShape = 'POSTER'

export interface CatalogTiles {
  items: PreviewItem[]
  /** The recipe shuffles: it takes a random TMDB page per call on the addon
   *  path, while preview always asks for page 1. Reported by the server rather
   *  than re-derived from params here. */
  randomized: boolean
  isLoading: boolean
  /** TMDB was unreachable, or the recipe was rejected. */
  isError: boolean
}

const EMPTY: CatalogTiles = { items: [], randomized: false, isLoading: false, isError: false }

/** What a view should render for a catalog it has no tiles for. Settled and
 *  empty, so it draws nothing rather than spinning forever. */
export function noTiles(): CatalogTiles {
  return EMPTY
}

/**
 * One folder, drawn as a tile at its own `tile_shape`.
 *
 * The tile face falls back in order: cover image, then cover emoji, then the
 * folder's title. Clicking opens the folder's page.
 *
 * A collection's row on home is made of these — folder metadata, never catalog
 * content. Folders in one collection can disagree about `tile_shape`, so the
 * row is legitimately ragged and is drawn that way: the raggedness is part of
 * what's being previewed, not a glitch to normalise away.
 */
export function FolderTile({ folder, onOpen }: { folder: PreviewFolder; onOpen: () => void }) {
  const height = 92
  const width = height * TILE_ASPECT[folder.tileShape]
  const name = folder.title || 'Untitled folder'

  const notes = [
    `${folder.sources.length} ${folder.sources.length === 1 ? 'catalog' : 'catalogs'}`,
  ]
  if (folder.unresolved > 0) notes.push(`${folder.unresolved} unavailable`)
  if (folder.tileShapeAssumed) notes.push('no shape set — shown as poster')

  return (
    <button
      type="button"
      onClick={onOpen}
      // The accessible name carries the folder's identity even when
      // `hide_title` suppresses the caption, so the tile never becomes an
      // anonymous target.
      aria-label={`Open ${name}`}
      title={`${name} — ${notes.join(' · ')}`}
      className="group flex shrink-0 flex-col gap-1.5 text-left"
    >
      <span
        style={{ width: `${width}px`, height: `${height}px` }}
        className="bg-raised border-line group-hover:border-line-hi relative grid shrink-0 place-items-center overflow-hidden rounded-[2px] border px-1 transition-colors"
      >
        {folder.coverEmoji ? (
          <span aria-hidden="true" className="text-[22px] leading-none">
            {folder.coverEmoji}
          </span>
        ) : (
          <span aria-hidden="true" className="type-data text-dimmer text-center text-[9px] leading-tight">
            {name}
          </span>
        )}
        {/* Layered over the emoji/title rather than replacing them, so a cover
            URL that 404s or is slow leaves a legible tile instead of a void. */}
        {folder.coverImageUrl && (
          <img
            src={folder.coverImageUrl}
            alt=""
            loading="lazy"
            className="absolute inset-0 h-full w-full object-cover"
          />
        )}
      </span>

      <span
        style={{ maxWidth: `${width}px` }}
        className="type-data text-dim truncate text-[10px]"
        aria-hidden="true"
      >
        {/* `hide_title` hides the tile's title text on the TV, so preview hides
            it too. The slot is kept — and says what happened — so the row's
            baseline doesn't jump between folders that hide titles and folders
            that don't. */}
        {folder.hideTitle ? <span className="text-dimmer">title hidden</span> : name}
      </span>
    </button>
  )
}

/**
 * A horizontal row of tiles, clipped at the pane's edge the way a real row runs
 * off the side of the screen.
 *
 * The clipped tiles cost nothing: a full TMDB page arrives whether seven or
 * twenty are drawn, and `loading="lazy"` on each poster means the ones past the
 * edge never fetch their image.
 */
export function TileStrip({ shape, tiles }: { shape: TileShape; tiles: CatalogTiles }) {
  const height = 92
  return (
    <TileRun tiles={tiles} width={height * TILE_ASPECT[shape]} height={height} wrap={false} />
  )
}

export function TileGrid({
  shape,
  tiles,
  kind,
}: {
  shape: TileShape
  tiles: CatalogTiles
  /** Set only where every tile in the run is the same kind, which is what
   *  makes a TMDB link constructible. Omitted, the tiles stay inert. */
  kind?: TMDBKind
}) {
  const width = 88
  return (
    <TileRun tiles={tiles} width={width} height={width / TILE_ASPECT[shape]} wrap kind={kind} />
  )
}

/**
 * The shared body of both layouts: real posters once they land, placeholders
 * until then.
 *
 * Placeholders are drawn at `TILES_PER_PAGE`, matching a full page, so the row
 * doesn't reflow when content arrives.
 */
export function TileRun({
  tiles,
  width,
  height,
  wrap,
  kind,
}: {
  tiles: CatalogTiles
  width: number
  height: number
  wrap: boolean
  kind?: TMDBKind
}) {
  const className = `flex gap-2 overflow-hidden ${wrap ? 'flex-wrap' : ''}`

  // A settled, empty result draws nothing at all — placeholders there would
  // read as perpetual loading, and the caller's own note has already said why
  // it's empty.
  if (!tiles.isLoading && !tiles.isError && tiles.items.length === 0) return null

  if (tiles.items.length === 0) {
    return (
      <div aria-hidden="true" className={className}>
        {Array.from({ length: TILES_PER_PAGE }, (_, i) => (
          <PlaceholderTile key={i} width={width} height={height} />
        ))}
      </div>
    )
  }

  return (
    <div className={className}>
      {tiles.items.map((item) => (
        <ContentTile key={item.tmdb_id} item={item} width={width} height={height} kind={kind} />
      ))}
    </div>
  )
}

/**
 * One real title. The title sits behind the poster rather than beside it, so a
 * title TMDB has no poster for degrades to a readable tile, not an empty box.
 *
 * **A link when the caller knows the kind, a plain tile otherwise.** `tmdb_id`
 * plus movie-or-tv is the whole of a themoviedb.org URL, and checking a title
 * the recipe returned is the obvious next question once the tiles are on
 * screen. It opens in a new tab: the builder holds unsaved form state, and
 * navigating away from it to read a synopsis would discard the work.
 */
export function ContentTile({
  item,
  width,
  height,
  kind,
}: {
  item: PreviewItem
  width: number
  height: number
  kind?: TMDBKind
}) {
  const name = item.year ? `${item.title} (${item.year})` : item.title
  const face = (
    <>
      <span className="type-data text-dimmer px-1 text-center text-[9px] leading-tight">
        {item.title}
      </span>
      {item.poster && (
        <img
          src={item.poster}
          alt=""
          loading="lazy"
          className="absolute inset-0 h-full w-full object-cover"
        />
      )}
    </>
  )
  const box = 'bg-raised border-line relative grid shrink-0 place-items-center overflow-hidden rounded-[2px] border'
  const style = { width: `${width}px`, height: `${height}px` }

  if (!kind) {
    return (
      <span title={name} style={style} className={box}>
        {face}
      </span>
    )
  }

  return (
    <a
      href={`https://www.themoviedb.org/${kind}/${item.tmdb_id}`}
      target="_blank"
      rel="noopener noreferrer"
      title={`${name} — open on TMDB`}
      style={style}
      className={`${box} hover:border-ink focus-visible:ring-ink cursor-pointer transition-colors outline-none focus-visible:ring-2`}
    >
      {face}
    </a>
  )
}

export function PlaceholderTile({ width, height }: { width: number; height: number }) {
  return (
    <span
      className="bg-raised border-line shrink-0 rounded-[2px] border"
      style={{ width: `${width}px`, height: `${height}px` }}
    />
  )
}

/**
 * What the tiles beside a heading couldn't say for themselves.
 *
 * Silent while loading — the placeholder tiles already carry that.
 *
 * The error wording assumes the Home pane's contract, where tiles are fetched
 * for the user rather than on request: it explains the gap and moves on. A
 * caller that fetched because someone pressed a button owes them a retry
 * instead, and says so itself.
 */
export function TilesNote({ tiles }: { tiles: CatalogTiles }) {
  if (tiles.isLoading) return null

  if (tiles.isError) {
    return (
      <span
        className="type-data text-dimmer shrink-0 text-[10px]"
        title="The catalog itself is fine — only this preview failed to load."
      >
        · couldn't load titles — showing layout only
      </span>
    )
  }

  if (tiles.items.length === 0) {
    return <span className="type-data text-dimmer shrink-0 text-[10px]">· nothing matches these filters right now</span>
  }

  if (tiles.randomized) {
    // The addon path takes a random TMDB page per call while preview always
    // takes page 1, so these specific titles are not what the TV will show.
    return (
      <span
        className="type-data text-dimmer shrink-0 text-[10px]"
        title="This catalog shuffles, so your TV gets a different set each time. These are a sample, not a prediction."
      >
        · shuffles — your TV will show a different set
      </span>
    )
  }

  return null
}
