import { useEffect, useMemo } from 'react'
import type { CertificationsByCountry, Genre, Language, TMDBParams } from '@/api'
import { EditorFooter, SaveError } from '@/features/builder/EditorFooter'
import { EditorShell } from '@/features/builder/EditorShell'
import { useEditorForm } from '@/features/builder/useEditorForm'
import { useRecipeTiles } from '@/features/preview/useRecipeTiles'
import type { CountryLookup } from './countries'
import {
  SORT_FIELDS,
  emptyForm,
  isSameCatalog,
  paramsString,
  parseGenreList,
  parseSortBy,
  serializeGenreList,
  serializeSortBy,
  validateForm,
  type BuilderMode,
  type CatalogFormState,
  type DateMode,
} from './catalogForm'
import { RecipePreview } from './RecipePreview'
import { WatchProviderPicker } from './WatchProviderPicker'
import {
  CertificationPicker,
  Checkbox,
  Field,
  FieldNote,
  GenreCycler,
  RangeField,
  Segmented,
  Select,
  TextInput,
} from './fields'

/**
 * Create / edit / duplicate a catalog, filling the builder's right pane.
 *
 * One component with a mode rather than separate create and edit shells: the
 * two differ only in whether `type` is editable, and `type` is what the params
 * sub-form branches on. Duplicate is create seeded from an existing row.
 *
 * **Dirtiness is reported, not handled.** Every way out of this editor
 * originates outside it — the × in the shell, Escape, selecting another row in
 * the rail, switching profile — so the pane owns the confirmation and this only
 * has to say whether there is anything to lose.
 */
export function CatalogEditor({
  mode,
  initial,
  genres,
  certifications,
  countryNames,
  languages,
  saving,
  serverError,
  onSave,
  onRequestClose,
  onDuplicate,
  onDelete,
  onDirtyChange,
}: {
  mode: BuilderMode
  initial: CatalogFormState | null
  genres: { movie: Genre[]; tv: Genre[] }
  certifications: { movie: CertificationsByCountry; tv: CertificationsByCountry }
  countryNames: CountryLookup
  languages: Language[]
  saving: boolean
  /** Plain-text body of a server 400. Should be unreachable — the form mirrors
   *  every rule — so it renders as an unexpected-case banner, not a field. */
  serverError: string | null
  onSave: (state: CatalogFormState) => void
  /** Also backs the mobile Library button below `lg` — see `EditorShell`. */
  onRequestClose: () => void
  /** This catalog's own row actions, carried in the header below `lg`. Absent
   *  while creating, when there is no row yet. */
  onDuplicate?: () => void
  onDelete?: () => void
  onDirtyChange: (dirty: boolean) => void
}) {
  const baseline = useMemo(() => initial ?? emptyForm(), [initial])
  const { state, setState, showErrors, revealErrors, submit } = useEditorForm(
    baseline,
    isSameCatalog,
    onDirtyChange,
  )

  // Sorted by the name shown, not TMDB's response order, so the dropdown
  // reads alphabetically like the country and certification pickers.
  const languageOptions = useMemo(
    () =>
      [...languages]
        .sort((a, b) => a.english_name.localeCompare(b.english_name))
        .map(({ iso_639_1, english_name }) => ({ value: iso_639_1, label: english_name })),
    [languages],
  )

  // Fed the recipe as it stands on every render, but only *fetches* when the
  // preview block's button is pressed — see `useRecipeTiles`. `name` and
  // `is_public` aren't part of a recipe, so renaming a catalog doesn't make
  // its preview stale.
  const preview = useRecipeTiles(state.type, paramsString(state))
  const resetPreview = preview.reset

  // Seeding a different catalog means the tiles on screen belong to the
  // previous one. Stale is the wrong word for that — they aren't this recipe's
  // results at all — so they go rather than being labelled.
  useEffect(() => {
    resetPreview()
  }, [baseline, resetPreview])

  const errors = useMemo(() => validateForm(state), [state])
  const errorCount = Object.keys(errors).length
  const errorFor = (key: string) => (showErrors ? errors[key] : undefined)

  // The form mirrors every server rule, so an invalid recipe never leaves the
  // browser: the server would answer 400 and the form already knows which
  // field it would name.
  //
  // `name` is excluded deliberately — it's the one error that isn't part of a
  // recipe. Tuning filters before deciding what to call the thing is the normal
  // order, and blocking preview on an empty name would invert it.
  const recipeInvalid = Object.keys(errors).some((key) => key !== 'name')

  // Genre ids differ between movie and tv, so the list swaps with `type` —
  // and any ids already picked are cleared, because they mean something
  // different (or nothing) in the other space. Only reachable while creating;
  // `type` is locked once a catalog exists.
  const activeGenres = state.type === 'movie' ? genres.movie : genres.tv
  const withGenres = parseGenreList(state.params.with_genres)
  const withoutGenres = parseGenreList(state.params.without_genres)

  const { field: sortField, direction: sortDirection } = parseSortBy(state.params.sort_by)

  // Same split as genres: movie and tv certifications are different scales
  // per country, even though both are scoped by the same certification_country.
  const activeCertifications = state.type === 'movie' ? certifications.movie : certifications.tv

  function patch(update: Partial<CatalogFormState>) {
    setState((previous) => ({ ...previous, ...update }))
  }

  function patchParams(update: Partial<TMDBParams>) {
    setState((previous) => ({ ...previous, params: { ...previous.params, ...update } }))
  }

  function changeType(type: CatalogFormState['type']) {
    setState((previous) => ({
      ...previous,
      type,
      dateMode: 'any',
      params: {
        ...previous.params,
        with_genres: undefined,
        without_genres: undefined,
        sort_by: undefined,
        // Movie and tv certifications are different scales per country, even
        // under the same certification_country — a rating picked for one
        // means nothing (or something else) in the other.
        certification: undefined,
        certification_gte: undefined,
        certification_lte: undefined,
        certification_country: undefined,
      },
    }))
  }

  /** Same shape as `submit`: reveal what's wrong, or go. Errors stay hidden
   *  until something is submitted, so pressing Preview has to be one of the
   *  things that reveals them — otherwise the note explaining why it won't run
   *  points at highlighting that isn't there yet. */
  function runPreview() {
    if (recipeInvalid) {
      revealErrors()
      return
    }
    preview.run()
  }

  const eyebrow =
    mode === 'edit' ? 'Edit catalog' : 'Duplicate catalog'

  return (
    <EditorShell
      eyebrow={eyebrow}
      title={state.name.trim() || 'Untitled catalog'}
      kind={state.type}
      // A duplicate is born yours whoever you copied it from, and so is a new
      // one — the only editable catalog is one you own.
      owned
      onRequestClose={onRequestClose}
      onDuplicate={onDuplicate}
      onDelete={onDelete}
      footer={
        <EditorFooter
          mode={mode}
          noun="catalog"
          saving={saving}
          showErrors={showErrors}
          errorCount={errorCount}
          onCancel={onRequestClose}
          onSubmit={() => submit(errorCount, onSave)}
        />
      }
    >
        <div className="grid gap-x-8 gap-y-6 md:grid-cols-[minmax(0,232px)_minmax(0,1fr)]">
          {/* --- identity ------------------------------------------------ */}
          <div className="flex flex-col gap-6">
            <Field label="Name" error={errorFor('name')}>
              <TextInput
                value={state.name}
                onChange={(name) => patch({ name })}
                placeholder="Trending Sci-Fi"
                invalid={Boolean(errorFor('name'))}
              />
            </Field>

            <Field
              label="Type"
              hint={mode === 'edit' ? 'Fixed once created — duplicate to change it.' : undefined}
            >
              {mode === 'edit' ? (
                <p className="type-data text-dim m-0 py-2 text-[13px]">
                  {state.type === 'movie' ? 'Movie' : 'Series'}
                </p>
              ) : (
                <Segmented
                  ariaLabel="Catalog type"
                  value={state.type}
                  onChange={changeType}
                  options={[
                    { value: 'movie', label: 'Movie' },
                    { value: 'series', label: 'Series' },
                  ]}
                />
              )}
            </Field>

            <Checkbox
              checked={state.isPublic}
              onChange={(isPublic) => patch({ isPublic })}
              label="Share with the community"
              hint="Others can add it to their own home screen."
            />
          </div>

          {/* --- filters -------------------------------------------------- */}
          <div className="flex flex-col gap-6">
            <Field label="Sort by" error={errorFor('sort_by')}>
              <div className="flex flex-wrap items-center gap-2">
                <Select
                  value={sortField}
                  onChange={(field) =>
                    patchParams({ sort_by: serializeSortBy(field, sortDirection) })
                  }
                  placeholder="Popularity"
                  options={SORT_FIELDS[state.type]}
                />
                <Segmented
                  ariaLabel="Sort direction"
                  value={sortDirection}
                  onChange={(direction) =>
                    patchParams({ sort_by: serializeSortBy(sortField || 'popularity', direction) })
                  }
                  options={[
                    { value: 'desc', label: 'High to low' },
                    { value: 'asc', label: 'Low to high' },
                  ]}
                />
              </div>
            </Field>

            <GenreCycler
              label="Genres"
              genres={activeGenres}
              withIds={withGenres.ids}
              withJoin={withGenres.join}
              withoutIds={withoutGenres.ids}
              onChange={(withIds, withJoin, withoutIds) =>
                patchParams({
                  with_genres: withIds.length ? serializeGenreList(withIds, withJoin) : undefined,
                  without_genres: withoutIds.length
                    ? serializeGenreList(withoutIds, 'and')
                    : undefined,
                })
              }
            />

            {/* `xl`, not `sm`: the breakpoint measures the viewport, and from
                `lg` up this grid sits in a pane that is the viewport minus a
                372px rail minus the identity column. Splitting at `sm` gave
                each half about 150px, which is narrower than the two number
                boxes inside it — they overflowed the column rather than
                wrapping.

                Below `lg` the rail is a screen of its own rather than a column
                beside this one, so the pane is the whole viewport and the
                mismatch doesn't arise — the grid is single-column there
                anyway. */}
            <div className="grid gap-x-8 gap-y-6 xl:grid-cols-2">
              <RangeField
                label="Rating"
                error={errorFor('vote_average')}
                low={state.params.vote_average_gte}
                high={state.params.vote_average_lte}
                onLow={(vote_average_gte) => patchParams({ vote_average_gte })}
                onHigh={(vote_average_lte) => patchParams({ vote_average_lte })}
                step="0.1"
                min={0}
                max={10}
              />
              <RangeField
                label="Number of ratings"
                error={errorFor('vote_count')}
                low={state.params.vote_count_gte}
                high={state.params.vote_count_lte}
                onLow={(vote_count_gte) => patchParams({ vote_count_gte })}
                onHigh={(vote_count_lte) => patchParams({ vote_count_lte })}
                step="10"
                min={0}
                max={5000}
                formatValue={(value) => (value >= 5000 ? '5000+' : String(value))}
              />
              <RangeField
                label="Runtime"
                unit="min"
                error={errorFor('with_runtime')}
                low={state.params.with_runtime_gte}
                high={state.params.with_runtime_lte}
                onLow={(with_runtime_gte) => patchParams({ with_runtime_gte })}
                onHigh={(with_runtime_lte) => patchParams({ with_runtime_lte })}
                step="5"
                min={0}
                max={300}
                formatValue={(value) => (value >= 300 ? '300+' : String(value))}
              />
              <Field label="Original language">
                <Select
                  value={state.params.with_original_language ?? ''}
                  onChange={(value) =>
                    patchParams({ with_original_language: value || undefined })
                  }
                  placeholder="Any"
                  options={languageOptions}
                />
                {languageOptions.length === 0 && (
                  <FieldNote>Couldn't load languages.</FieldNote>
                )}
              </Field>
            </div>

            <DateWindow
              state={state}
              error={errorFor('within_days')}
              onMode={(dateMode) => patch({ dateMode })}
              onParams={patchParams}
            />

            <CertificationPicker
              label="Age rating"
              tip="Ratings differ by country, so pick one first. The slider then sets the lowest and highest rating allowed."
              countries={activeCertifications}
              countryNames={countryNames}
              country={state.params.certification_country}
              gte={state.params.certification_gte ?? state.params.certification}
              lte={state.params.certification_lte ?? state.params.certification}
              error={errorFor('certification_country')}
              onChange={(update) => patchParams({ ...update, certification: undefined })}
            />

            <WatchProviderPicker
              type={state.type}
              params={state.params}
              error={errorFor('watch_region')}
              onParams={patchParams}
            />

            <Checkbox
              checked={Boolean(state.params.randomized)}
              onChange={(randomized) => patchParams({ randomized: randomized || undefined })}
              label="Shuffle results"
              hint="Shows a different set of titles each time the row opens."
            />
          </div>
        </div>

        <RecipePreview
          preview={preview}
          type={state.type}
          invalid={recipeInvalid}
          onRun={runPreview}
        />

        <SaveError noun="catalog" message={serverError} />
    </EditorShell>
  )
}

/**
 * Fixed range XOR rolling window, as one mode toggle. The server rejects both
 * being set; making the mode the control means that state can't be expressed.
 */
function DateWindow({
  state,
  error,
  onMode,
  onParams,
}: {
  state: CatalogFormState
  error?: string
  onMode: (mode: DateMode) => void
  onParams: (update: Partial<TMDBParams>) => void
}) {
  const isMovie = state.type === 'movie'
  const label = isMovie ? 'Release date' : 'First aired'
  const gte = isMovie ? state.params.primary_release_date_gte : state.params.first_air_date_gte
  const lte = isMovie ? state.params.primary_release_date_lte : state.params.first_air_date_lte
  const days = isMovie ? state.params.released_within_days : state.params.aired_within_days

  return (
    <div className="flex flex-col gap-2">
      {/* The mode toggle sits beside its label rather than at the far edge of
          the column: pushed apart by a `flex-1` label the two stopped reading
          as one control. Same for Genres below. */}
      <div className="flex flex-wrap items-center gap-3">
        <label className="type-eyebrow">{label}</label>
        <Segmented
          ariaLabel={`${label} mode`}
          value={state.dateMode}
          onChange={onMode}
          options={[
            { value: 'any', label: 'Any' },
            { value: 'fixed', label: 'Range' },
            { value: 'rolling', label: 'Recent' },
          ]}
        />
      </div>

      {state.dateMode === 'fixed' && (
        <div className="flex items-center gap-2">
          <input
            type="date"
            value={gte ?? ''}
            onChange={(event) =>
              onParams(
                isMovie
                  ? { primary_release_date_gte: event.target.value || undefined }
                  : { first_air_date_gte: event.target.value || undefined },
              )
            }
            className="field type-data w-full max-w-[var(--w-date)]"
          />
          <span className="text-dimmer shrink-0 text-[12.5px]">–</span>
          <input
            type="date"
            value={lte ?? ''}
            onChange={(event) =>
              onParams(
                isMovie
                  ? { primary_release_date_lte: event.target.value || undefined }
                  : { first_air_date_lte: event.target.value || undefined },
              )
            }
            className="field type-data w-full max-w-[var(--w-date)]"
          />
        </div>
      )}

      {state.dateMode === 'rolling' && (
        <RollingWindow
          isMovie={isMovie}
          days={days}
          error={error}
          onDays={(value) =>
            onParams(isMovie ? { released_within_days: value } : { aired_within_days: value })
          }
        />
      )}
    </div>
  )
}

/**
 * The windows people actually ask for, as a row of presets rather than a box
 * that wants a number of days.
 *
 * "90" is not a thing anyone thinks in — they think "the last three months" —
 * and a bare number field made the user do the arithmetic and then guess
 * whether they'd got it right. Each preset is still just `_within_days` on the
 * wire, so nothing about the recipe or its validation changes; only the way
 * the number is arrived at.
 */
const DATE_PRESETS: { days: number; label: string }[] = [
  { days: 30, label: '30 days' },
  { days: 90, label: '90 days' },
  { days: 182, label: '6 months' },
  { days: 365, label: '1 year' },
]

/**
 * Upcoming is the same rolling filter with the window closed up to yesterday.
 *
 * `_within_days` becomes a `.gte` and nothing else — there is no upper bound —
 * so a one-day window is "dated yesterday or later", which over a discover
 * page sorted by popularity is the unreleased slate. It is a day wider than
 * the word promises, and it recalculates daily like every other preset; the
 * alternative, a fixed `gte` pinned to the day the catalog was saved, would
 * read as "upcoming" for one day and then quietly rot.
 */
const UPCOMING_DAYS = 1

const DAYS_PER_YEAR = 365

function RollingWindow({
  isMovie,
  days,
  error,
  onDays,
}: {
  isMovie: boolean
  days: number | undefined
  error?: string
  onDays: (days: number | undefined) => void
}) {
  const isPreset = DATE_PRESETS.some((option) => option.days === days)
  const isUpcoming = days === UPCOMING_DAYS
  // Only an exact number of years reads back into the box — 90 days is the
  // preset's business, and showing "0" there would invite editing it into
  // something the chip above already covers.
  const customYears =
    days !== undefined && days > DAYS_PER_YEAR && days % DAYS_PER_YEAR === 0
      ? days / DAYS_PER_YEAR
      : undefined

  // A window none of the controls above can show: catalogs built before the
  // presets existed carry whatever number was typed into the old day box, and
  // 45 is neither a preset nor a whole year. Without a chip of its own it
  // would render as nothing selected while 45 was still in the payload — and
  // the first chip pressed would silently overwrite a setting the user never
  // saw. Shown, selected, and replaced only on purpose.
  const oddDays =
    days !== undefined && !isPreset && !isUpcoming && customYears === undefined ? days : undefined

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-1.5">
        {DATE_PRESETS.map((option) => (
          <PresetChip
            key={option.days}
            label={option.label}
            selected={option.days === days}
            onClick={() => onDays(option.days)}
          />
        ))}
        <PresetChip
          label="Upcoming"
          selected={isUpcoming}
          onClick={() => onDays(UPCOMING_DAYS)}
        />
        {oddDays !== undefined && (
          <PresetChip
            label={`${oddDays} days`}
            selected
            onClick={() => onDays(oddDays)}
          />
        )}

        <span className="bg-line mx-1 h-4 w-px shrink-0" aria-hidden="true" />

        <label className="flex items-center gap-1.5">
          <span className="sr-only">Custom window, in years</span>
          <input
            type="number"
            min={1}
            max={50}
            value={customYears ?? ''}
            placeholder="#"
            onChange={(event) => {
              const years = Number(event.target.value)
              onDays(
                event.target.value === '' || years < 1
                  ? undefined
                  : years * DAYS_PER_YEAR,
              )
            }}
            className={`field type-data w-[3.25rem] px-2 py-1 text-center text-[12px] pointer-coarse:w-[4rem] pointer-coarse:text-[16px] ${
              customYears !== undefined ? 'border-dim text-ink' : ''
            }`}
          />
          <span className="type-data text-dimmer text-[10.5px]">years</span>
        </label>
      </div>

      {error && <FieldNote tone="danger">{error}</FieldNote>}

      {/* The date the chip resolves to today. The window is recomputed
          server-side per request, so this moves with the calendar — which is
          the one thing the chip's own label can't say. */}
      {days !== undefined && !error && (
        <FieldNote>
          {isUpcoming
            ? `${isMovie ? 'Released' : 'Airing'} from ${formatWindowStart(UPCOMING_DAYS)} onward, updated daily.`
            : `${isMovie ? 'Released' : 'Aired'} since ${formatWindowStart(days)}, updated daily.`}
        </FieldNote>
      )}
    </div>
  )
}

function PresetChip({
  label,
  selected,
  onClick,
}: {
  label: string
  selected: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={selected}
      className={`rounded-[2px] border px-2.5 py-1 text-[11px] whitespace-nowrap transition-colors pointer-coarse:py-2 ${
        selected
          ? 'bg-raised-hi border-dim text-ink'
          : 'border-line text-dim hover:border-dim hover:text-ink'
      }`}
    >
      {label}
    </button>
  )
}

/** The date the server's `daysAgo` would produce for this window today. */
function formatWindowStart(days: number): string {
  const start = new Date()
  start.setDate(start.getDate() - days)
  return start.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
}
