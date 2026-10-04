import type { PreviewItem, TileShape, TMDBKind } from '@/api'

/**
 * Drawing a catalog's content in Uno's own look: the tiles themselves and the
 * shapes they're drawn at.
 *
 * Drawn by the catalog editor's results panel; the Home pane's Preview and the
 * collection editor's folder tree share its types and shapes, which is why it
 * lives outside `features/home`. The preview rows draw
 * their own captioned tiles (`features/home/previewScreen.tsx`). Everything here is presentational
 * and takes `CatalogTiles` — how those tiles were fetched (for a saved
 * catalog, or for a recipe still being typed) is the caller's problem.
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
 * an assumption — the convention in every client in this space.
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
 * Real posters once they land, placeholders until then, in a grid whose tiles
 * stretch to fill the column edge to edge (`.tile-grid`): as many columns as
 * fit at 104px or wider, the width the preview rows' posters are drawn at.
 *
 * Placeholders are drawn at `TILES_PER_PAGE`, matching a full page, so the grid
 * doesn't reflow when content arrives.
 */
export function TileGrid({
  shape,
  tiles,
  kind,
}: {
  shape: TileShape
  tiles: CatalogTiles
  kind?: TMDBKind
}) {
  return <TileRun tiles={tiles} style={{ aspectRatio: TILE_ASPECT[shape] }} kind={kind} />
}

function TileRun({ tiles, style, kind }: { tiles: CatalogTiles; style: TileStyle; kind?: TMDBKind }) {
  if (settledEmpty(tiles)) return null
  if (tiles.items.length === 0) return <Placeholders style={style} />

  return (
    <div className="tile-grid">
      {tiles.items.map((item) => (
        <ContentTile key={item.tmdb_id} item={item} style={style} kind={kind} />
      ))}
    </div>
  )
}


/** A settled, empty result draws nothing at all — placeholders there would
 *  read as perpetual loading, and the caller's own note has already said why
 *  it's empty. */
function settledEmpty(tiles: CatalogTiles): boolean {
  return !tiles.isLoading && !tiles.isError && tiles.items.length === 0
}

function Placeholders({ style }: { style: TileStyle }) {
  return (
    <div aria-hidden="true" className="tile-grid">
      {Array.from({ length: TILES_PER_PAGE }, (_, i) => (
        <PlaceholderTile key={i} style={style} />
      ))}
    </div>
  )
}

type TileStyle = { aspectRatio: number }

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
function ContentTile({
  item,
  style,
  kind,
}: {
  item: PreviewItem
  style: TileStyle
  kind?: TMDBKind
}) {
  const name = item.year ? `${item.title} (${item.year})` : item.title
  const face = (
    <>
      <span className="type-data text-dimmer px-1 text-center text-[11px] leading-tight">
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
  const box = 'bg-raised border-line relative grid min-w-0 place-items-center overflow-hidden rounded-md border'

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

function PlaceholderTile({ style }: { style: TileStyle }) {
  return <span className="bg-raised border-line rounded-md border" style={style} />
}
