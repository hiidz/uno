import { CONTENT_TILE_SHAPE, TileGrid } from '@/features/preview/tiles'
import type { RecipePreview as Preview } from '@/features/preview/useRecipeTiles'

/**
 * What the filters above actually return, on request.
 *
 * **Nothing is fetched until the button is pressed.** The recipe changes on
 * every keystroke, so fetching as you type would mean a request per character
 * and tiles that never settle long enough to read. Pressing it again after a
 * change re-runs it.
 *
 * The whole block is one TMDB page — the same page the row itself is, so this
 * is the row, not a sample of it. The exception is a shuffling recipe, which
 * takes a random page per call on the addon path and is labelled as such.
 */
export function RecipePreview({
  preview,
  invalid,
  onRun,
}: {
  preview: Preview
  /** The form has errors, so there is no valid recipe to run. The server would
   *  reject it, and the form already knows why. */
  invalid: boolean
  /** Runs the preview, or reveals the errors standing in its way. Never
   *  nothing: a disabled button beside "fix the highlighted fields" is a dead
   *  end while the fields aren't highlighted yet — the form only reveals its
   *  errors once something has been submitted. Pressing is what reveals them. */
  onRun: () => void
}) {
  return (
    <section className="border-line mt-6 flex flex-col gap-3 border-t pt-5">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <span className="type-eyebrow flex-1">What this returns</span>
        <Notes preview={preview} invalid={invalid} />
        <button
          type="button"
          onClick={onRun}
          disabled={preview.tiles.isLoading}
          className="btn-ghost"
        >
          {preview.tiles.isLoading
            ? 'Loading…'
            : preview.idle
              ? 'Preview results'
              : 'Preview again'}
        </button>
      </div>

      <Body preview={preview} invalid={invalid} />
    </section>
  )
}

/** The one-liners that sit beside the button: why it can't run, or what's true
 *  about the tiles below it. At most one applies at a time. */
function Notes({ preview, invalid }: { preview: Preview; invalid: boolean }) {
  if (invalid) {
    return (
      <span className="type-data text-dimmer text-[10.5px]">
        fix the highlighted fields to preview
      </span>
    )
  }

  if (preview.isStale) {
    return (
      <span
        className="type-data text-series text-[10.5px]"
        title="These tiles came from the filters as they were when you last pressed the button."
      >
        filters changed — preview again
      </span>
    )
  }

  if (preview.tiles.randomized) {
    return (
      <span
        className="type-data text-dimmer text-[10.5px]"
        title="This catalog shuffles: it picks a random TMDB page each time Nuvio loads. Preview shows page 1."
      >
        shuffles — Nuvio will show a different page each time
      </span>
    )
  }

  return null
}

function Body({ preview, invalid }: { preview: Preview; invalid: boolean }) {
  if (preview.idle) {
    return (
      <p className="type-data text-dimmer m-0 text-[11px]">
        {invalid
          ? 'Nothing to run yet — the filters above have something the server would reject.'
          : 'Nothing fetched yet. Press "Preview results" to see what content these filters will display.'}
      </p>
    )
  }

  // Unlike the Home pane, someone is standing here waiting for a result they
  // asked for, so a failed fetch is stated outright rather than degraded past.
  if (preview.tiles.isError) {
    return (
      <p className="type-data text-danger border-danger m-0 border-l-2 pl-3 text-[11px]">
        Couldn't reach TMDB. Your filters are fine — this is the preview's own fetch. Try again.
      </p>
    )
  }

  // The most useful thing this block can say, so it says it loudly. The Home
  // pane renders a settled-empty catalog as nothing at all, which is right
  // there and wrong here: an empty result is precisely the answer someone
  // pressed the button to get.
  if (!preview.tiles.isLoading && preview.tiles.items.length === 0) {
    return (
      <div className="border-series bg-series/5 flex flex-col gap-1.5 rounded-[2px] border border-l-2 px-4 py-3.5">
        <span className="text-[13px] font-medium">Nothing matches these filters.</span>
        <p className="type-data text-dim m-0 text-[11px]">
          TMDB has no titles matching these filters. This row would be empty in Nuvio. Loosen a filter and try again.
        </p>
      </div>
    )
  }

  return <TileGrid shape={CONTENT_TILE_SHAPE} tiles={preview.tiles} />
}
