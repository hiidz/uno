import { tmdbKind } from '@/api'
import type { CatalogType } from '@/api'
import { CONTENT_TILE_SHAPE, TileGrid } from '@/features/preview/tiles'
import type { RecipePreview as Preview } from '@/features/preview/useRecipeTiles'
import { plural } from '@/lib/plural'

interface RecipePreviewProps {
  preview: Preview
  /** Which half of themoviedb.org a tile belongs to. The recipe's own type;
   *  every tile in one run shares it. */
  type: CatalogType
  /** The form has errors, so there is no valid recipe to run. The server would
   *  reject it, and the form already knows why. */
  invalid: boolean
  /** Runs the preview, or reveals the errors standing in its way. Never
   *  nothing: a disabled button beside "fix the highlighted fields" is a dead
   *  end while the fields aren't highlighted yet — the form only reveals its
   *  errors once something has been submitted. Pressing is what reveals them. */
  onRun: () => void
  /** A recipe nobody is editing here: there is no filter to loosen. */
  readOnly?: boolean
}

/**
 * What the filters above actually return, on request — DESIGN.md's "Results
 * panel", docked right of the form and sticky at the pane's top.
 *
 * **Nothing is fetched until the button is pressed.** The recipe changes on
 * every keystroke, so fetching as you type would mean a request per character
 * and tiles that never settle long enough to read. Pressing it again after a
 * change re-runs it.
 *
 * The whole block is one TMDB page — the same page the row itself is, so this
 * is the row, not a sample of it. A shuffling recipe takes a random page per
 * run, as the row does in Nuvio.
 *
 * Every tile links to its TMDB page, which is where the question a preview
 * raises — "what *is* that one?" — gets answered.
 */
export function RecipePreview({ preview, type, invalid, onRun, readOnly }: RecipePreviewProps) {
  return (
    <aside className="ed-pv" aria-label="Results">
      <div className="ed-pv-head">
        <span className="type-label">One page of results</span>
        <button type="button" onClick={onRun} disabled={preview.tiles.isLoading} className="btn-secondary btn-sm">
          {preview.tiles.isLoading ? 'Running…' : preview.idle ? 'Run preview' : 'Run again'}
        </button>
      </div>

      <Body preview={preview} type={type} invalid={invalid} readOnly={readOnly} />
    </aside>
  )
}

/** What an empty result says: that the row would be empty, and for a recipe
 *  being edited, what to do about it. */
function EmptyResult({ readOnly }: { readOnly?: boolean }) {
  if (readOnly) return <p>Nothing matches these filters.</p>
  return <p>Nothing matches these filters. This row would be empty — loosen a filter and try again.</p>
}

interface BodyProps {
  preview: Preview
  type: CatalogType
  invalid: boolean
  readOnly: boolean | undefined
}

function Body({ preview, type, invalid, readOnly }: BodyProps) {
  if (invalid) {
    return <p>Fix the highlighted filters first, then run the preview.</p>
  }

  if (preview.idle) {
    return null
  }

  return <Outcome preview={preview} type={type} readOnly={readOnly} />
}

/** What a run came back with: its failure, an empty answer, or the tiles. */
function Outcome({ preview, type, readOnly }: Omit<BodyProps, 'invalid'>) {
  // Unlike the Home pane, someone is standing here waiting for a result they
  // asked for, so a failed fetch is stated outright rather than degraded past.
  if (preview.tiles.isError) {
    return <LoadFailed />
  }

  // The most useful thing this block can say, so it says it loudly. The Home
  // pane renders a settled-empty catalog as nothing at all, which is right
  // there and wrong here: an empty result is precisely the answer someone
  // pressed the button to get.
  if (!preview.tiles.isLoading && preview.tiles.items.length === 0) {
    return <EmptyResult readOnly={readOnly} />
  }

  return <Results preview={preview} type={type} />
}

function LoadFailed() {
  return (
    <p className="text-danger">
      Couldn’t load the preview. Your filters are fine — try again.
    </p>
  )
}

function Results({ preview, type }: { preview: Preview; type: CatalogType }) {
  return (
    <>
      {/* The filters have moved on since these tiles were fetched — in ink,
       *  never Nuvio yellow (DESIGN.md's Words Beside Colour Rule): nothing here
       *  is waiting for Nuvio the way an unpushed change is. */}
      {preview.isStale && (
        <p className="pv-stale">Your filters changed since this ran. Run it again to see the new results.</p>
      )}
      {preview.totalResults !== null && (
        <p>
          <span className="type-data">{preview.totalResults.toLocaleString()}</span>{' '}
          {plural(preview.totalResults, 'result')} for these filters.
        </p>
      )}
      <TileGrid shape={CONTENT_TILE_SHAPE} tiles={preview.tiles} kind={tmdbKind(type)} />
    </>
  )
}
