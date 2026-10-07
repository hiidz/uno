import type { TMDBParams } from '@/api'
import { FieldNote, Segmented } from '@/components/fields'
import { pluralCount } from '@/lib/plural'
import type { DateMode } from './catalogForm'
import { DATE_PRESETS, DAYS_PER_YEAR, RECENT_DAYS, UPCOMING_DAYS } from './params'
import { formatWindowStart } from './summary'

/** The setters the date window is handed. */
type ParamsPatch = Partial<TMDBParams>
type PatchParams = (update: ParamsPatch) => void
type SetDateMode = (mode: DateMode) => void
type SetDays = (days: number) => void

/** A stored window none of the presets is, in the units it reads in: "3
 *  years", "45 days". */
function windowLabel(days: number): string {
  return days % DAYS_PER_YEAR === 0 ? pluralCount(days / DAYS_PER_YEAR, 'year') : pluralCount(days, 'day')
}

/** What the date window's control shows: `DateMode`, with Upcoming — a
 *  one-day rolling window — told apart from the recent ones. */
type DateView = 'any' | 'recent' | 'upcoming' | 'fixed'

const DATE_VIEWS: { value: DateView; label: string }[] = [
  { value: 'any', label: 'Any time' },
  { value: 'recent', label: 'Recent' },
  { value: 'upcoming', label: 'Upcoming' },
  { value: 'fixed', label: 'Dates' },
]

const VIEW_MODE: Record<DateView, DateMode> = {
  any: 'any',
  recent: 'rolling',
  upcoming: 'rolling',
  fixed: 'fixed',
}

function dateView(mode: DateMode, days: number | undefined): DateView {
  if (mode !== 'rolling') return mode
  return days === UPCOMING_DAYS ? 'upcoming' : 'recent'
}

/** The window Recent opens on: the one already set, else `RECENT_DAYS`.
 *  Upcoming's one-day window is not a recent one. */
function recentDays(days: number | undefined): number {
  return days && days !== UPCOMING_DAYS ? days : RECENT_DAYS
}

interface DateWindowProps {
  isMovie: boolean
  mode: DateMode
  /** This type's own fixed bounds and rolling window: the release date's for
   *  a movie, the first air date's for a series. */
  gte: string | undefined
  lte: string | undefined
  days: number | undefined
  error?: string
  onMode: SetDateMode
  onParams: PatchParams
}

/**
 * When a title came out, as one control: any time, a recent window that moves
 * with the calendar, the upcoming slate, or fixed dates. The server rejects a
 * fixed range and a rolling window together; making the choice the control
 * means that state can't be expressed.
 *
 * Recent and Upcoming are both `_within_days` on the wire. "Upcoming" is the
 * rolling filter closed up to yesterday: a one-day window is "dated yesterday
 * or later", which over a discover page sorted by popularity is the unreleased
 * slate. It recalculates daily, unlike a fixed `gte` pinned to the day the
 * catalog was saved, which would read as "upcoming" for one day and then
 * quietly rot.
 */
export function DateWindow({
  isMovie,
  mode,
  gte,
  lte,
  days,
  error,
  onMode,
  onParams,
}: DateWindowProps) {
  const view = dateView(mode, days)

  function setDays(value: number) {
    onParams(isMovie ? { released_within_days: value } : { aired_within_days: value })
  }

  function choose(next: DateView) {
    onMode(VIEW_MODE[next])
    if (next === 'recent') setDays(recentDays(days))
    if (next === 'upcoming') setDays(UPCOMING_DAYS)
  }

  return (
    <>
      <Segmented<DateView> ariaLabel="Date window" value={view} onChange={choose} options={DATE_VIEWS} />
      <WindowBody
        view={view}
        isMovie={isMovie}
        gte={gte}
        lte={lte}
        days={days}
        error={error}
        onParams={onParams}
        onDays={setDays}
      />
      <FixedDatesError view={view} error={error} />
    </>
  )
}

/** The fixed dates' own error, under their two boxes; the recent window shows
 *  its error beside its chips. */
function FixedDatesError({ view, error }: { view: DateView; error?: string }) {
  if (view !== 'fixed' || !error) return null
  return <FieldNote tone="danger">{error}</FieldNote>
}

interface WindowBodyProps {
  view: DateView
  isMovie: boolean
  gte: string | undefined
  lte: string | undefined
  days: number | undefined
  error?: string
  onParams: PatchParams
  onDays: SetDays
}

/** What follows the date window's control: the two dates, the recent windows,
 *  or the date Upcoming starts from today. */
function WindowBody({
  view,
  isMovie,
  gte,
  lte,
  days,
  error,
  onParams,
  onDays,
}: WindowBodyProps) {
  if (view === 'fixed') return <FixedDates isMovie={isMovie} gte={gte} lte={lte} onParams={onParams} />
  if (view === 'recent') return <RecentWindow isMovie={isMovie} days={days} error={error} onDays={onDays} />
  if (view === 'upcoming') {
    return (
      <p className="ed-dateline">
        {isMovie ? 'Released' : 'Airing'} from {formatWindowStart(UPCOMING_DAYS)} onward, updated daily.
      </p>
    )
  }
  return null
}

interface FixedDatesProps {
  isMovie: boolean
  gte: string | undefined
  lte: string | undefined
  onParams: PatchParams
}

function FixedDates({
  isMovie,
  gte,
  lte,
  onParams,
}: FixedDatesProps) {
  return (
    <div className="ed-dates">
      <label className="setting-label type-label" htmlFor="ed-from">
        From
      </label>
      <input
        id="ed-from"
        type="date"
        value={gte ?? ''}
        onChange={(event) =>
          onParams(
            isMovie
              ? { primary_release_date_gte: event.target.value || undefined }
              : { first_air_date_gte: event.target.value || undefined },
          )
        }
        className="field type-data"
      />
      <label className="setting-label type-label" htmlFor="ed-to">
        To
      </label>
      <input
        id="ed-to"
        type="date"
        value={lte ?? ''}
        onChange={(event) =>
          onParams(
            isMovie
              ? { primary_release_date_lte: event.target.value || undefined }
              : { first_air_date_lte: event.target.value || undefined },
          )
        }
        className="field type-data"
      />
    </div>
  )
}

interface RecentWindowProps {
  isMovie: boolean
  days: number | undefined
  error?: string
  onDays: SetDays
}

/**
 * The recent windows people actually ask for, as a row of preset chips
 * (`DATE_PRESETS`, `params.ts`) rather than a box that wants a number of days —
 * "90" is not a thing anyone thinks in. Each is still just `_within_days` on
 * the wire.
 *
 * A stored window that is no preset — catalogs built before the presets
 * carry whatever number was typed then — gets a chip of its own, shown
 * selected and replaced only on purpose. Without it the row would show
 * nothing selected while the number was still in the payload, and the first
 * chip pressed would silently overwrite a setting nobody saw.
 */
function RecentWindow({
  isMovie,
  days,
  error,
  onDays,
}: RecentWindowProps) {
  const odd = days !== undefined && !DATE_PRESETS.some((option) => option.days === days) ? days : undefined

  return (
    <>
      <div className="choices" role="group" aria-label="How recent">
        {DATE_PRESETS.map((option) => (
          <button
            key={option.days}
            type="button"
            className="choice"
            aria-pressed={option.days === days}
            onClick={() => onDays(option.days)}
          >
            {option.label}
          </button>
        ))}
        {odd !== undefined && (
          <button type="button" className="choice" aria-pressed onClick={() => onDays(odd)}>
            {windowLabel(odd)}
          </button>
        )}
      </div>

      {error && <FieldNote tone="danger">{error}</FieldNote>}

      {/* The date the chip resolves to today. The window is recomputed
          server-side per request, so this moves with the calendar — which is
          the one thing the chip's own label can't say. */}
      {days !== undefined && !error && (
        <p className="ed-dateline">
          {isMovie ? 'Released' : 'Aired'} since {formatWindowStart(days)}, updated daily.
        </p>
      )}
    </>
  )
}

