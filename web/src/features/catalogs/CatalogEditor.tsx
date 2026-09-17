import { useEffect, useMemo, useState } from 'react'
import { ChevronDown, TriangleAlert } from 'lucide-react'
import type { CertificationsByCountry, Genre, Language, TMDBParams } from '@/api'
import { Icon } from '@/components/Icon'
import { EditorFooter, SaveError } from '@/features/builder/EditorFooter'
import { EditorShell } from '@/features/builder/EditorShell'
import { useEditorForm } from '@/features/builder/useEditorForm'
import { useRecipeTiles } from '@/features/preview/useRecipeTiles'
import { pluralCount } from '@/lib/plural'
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
import {
  DATE_PRESETS,
  DAYS_PER_YEAR,
  UPCOMING_DAYS,
  formatWindowStart,
  sumAge,
  sumDate,
  sumGenres,
  sumLanguage,
  sumOrder,
  sumRatings,
  sumShuffle,
  sumWatch,
} from './summary'
import { WatchProviderPicker } from './WatchProviderPicker'
import {
  CertificationPicker,
  FieldNote,
  GenreCycler,
  RangeField,
  Segmented,
  Select,
  Switch,
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
 *
 * **There is no read-only "imported" view.** The closed-graph sharing model
 * has no such state: a taken catalog is a private copy you fully own from
 * the moment it's created, not a live pointer
 * that could ever need a read-only screen. Every row this editor opens is
 * yours, so `BuilderMode` is `'edit' | 'duplicate'` with no third "viewing"
 * mode — `'duplicate'` is reached only through the explicit Duplicate action
 * on one of your own rows, never by opening one.
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
  const dirty = !isSameCatalog(baseline, state)

  // One section open at a time (DESIGN.md's "Collapsible sections as
  // credit-row buttons"). Every body stays mounted regardless — toggled with
  // `hidden`, not unmounted — because `WatchProviderPicker` keeps its own
  // region/search state locally and losing it every time the section closes
  // would mean re-picking a region on every reopen.
  const [openSection, setOpenSection] = useState<SectionKey | null>(null)
  const [genresExpanded, setGenresExpanded] = useState(false)

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
  const activeScale = state.params.certification_country
    ? (activeCertifications[state.params.certification_country] ?? [])
    : []
  const isMovie = state.type === 'movie'
  const dateGte = isMovie ? state.params.primary_release_date_gte : state.params.first_air_date_gte
  const dateLte = isMovie ? state.params.primary_release_date_lte : state.params.first_air_date_lte
  const dateDays = isMovie ? state.params.released_within_days : state.params.aired_within_days
  const watchProviderCount = state.params.with_watch_providers
    ? state.params.with_watch_providers.split(/[,|]/).filter(Boolean).length
    : 0

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

  /** DESIGN.md: pressing a greyed Save "takes focus to the field that needs
   *  fixing" — with one section open at a time, that field may be behind a
   *  closed head, so this opens it first. `name` sits outside every section. */
  function focusFirstError() {
    const firstKey = ERROR_PRIORITY.find((key) => key in errors)
    if (!firstKey) return
    if (firstKey === 'name') {
      document.getElementById('cat-name')?.focus()
      return
    }
    const section = ERROR_SECTION[firstKey]
    if (!section) return
    setOpenSection(section)
    requestAnimationFrame(() => document.getElementById(`sec-head-${section}`)?.focus())
  }

  function trySubmit() {
    if (errorCount > 0) {
      revealErrors()
      focusFirstError()
      return
    }
    submit(errorCount, onSave)
  }

  const sections = buildSections({
    state,
    sortField,
    sortDirection,
    activeGenres,
    withGenres,
    withoutGenres,
    genresExpanded,
    setGenresExpanded,
    languageOptions,
    languages,
    isMovie,
    dateGte,
    dateLte,
    dateDays,
    activeCertifications,
    countryNames,
    activeScale,
    watchProviderCount,
    errorFor,
    patch,
    patchParams,
  })

  const roleLabels = showErrors
    ? Array.from(
        new Set(Object.keys(errors).map((key) => roleLabelFor(key, isMovie))),
      )
    : []

  const status =
    errorCount > 0 ? (
      <span className="ed-status is-error">
        <Icon icon={TriangleAlert} size={16} />
        {pluralCount(errorCount, 'thing')} {errorCount === 1 ? 'needs' : 'need'} fixing:{' '}
        {roleLabels.join(', ')}
      </span>
    ) : dirty ? (
      <span className="ed-status">Unsaved changes</span>
    ) : (
      <span className="ed-status is-muted">No changes yet</span>
    )

  const isBare = paramsString(state) === '{}'

  return (
    <EditorShell
      eyebrow={mode === 'edit' ? 'Edit catalog' : 'Duplicate catalog'}
      title={state.name.trim() || 'Untitled catalog'}
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
          onSubmit={trySubmit}
          status={status}
          saveLabel="Save"
        />
      }
    >
      <div className="ed-container">
        <div className="ed">
          <div className="ed-form">
            <div className="cr is-field">
              <label htmlFor="cat-name" className="cr-role type-eyebrow">
                Name
              </label>
              <div className="cr-val">
                <TextInput
                  id="cat-name"
                  value={state.name}
                  onChange={(name) => patch({ name })}
                  placeholder="Trending Sci-Fi"
                  invalid={Boolean(errorFor('name'))}
                />
                {errorFor('name') && (
                  <p className="field-error">
                    <Icon icon={TriangleAlert} size={16} className="text-danger" />
                    <span>{errorFor('name')}</span>
                  </p>
                )}
              </div>
            </div>

            <div className="cr">
              <span className="cr-role type-eyebrow">Movies or series</span>
              <div className="cr-val">
                {mode === 'edit' ? (
                  <span className="type-data text-[15px]">
                    {state.type === 'movie' ? 'Movie' : 'Series'}{' '}
                    <span className="aside">
                      · locked. Duplicate it to make a {state.type === 'movie' ? 'series' : 'movie'}{' '}
                      version.
                    </span>
                  </span>
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
              </div>
            </div>

            {state.collectionID !== null ? (
              <div className="cr">
                <span className="cr-role type-eyebrow">Scope</span>
                <div className="cr-val ed-line">
                  <span className="type-data text-[13px]">Only inside this collection</span>
                  <button
                    type="button"
                    className="btn-secondary btn-sm"
                    onClick={() => patch({ collectionID: null, isPublic: false })}
                  >
                    Move to library
                  </button>
                </div>
                <p className="type-data text-dimmer m-0 pt-1 text-[11px] leading-[1.45]">
                  Scoped catalogs can't be shared. Moving it to your library makes it usable from
                  any of your folders and lets it join the community list — takes effect on Save.
                </p>
              </div>
            ) : (
              <div className="cr is-switch">
                <span className="cr-role type-eyebrow">Sharing</span>
                <div className="cr-val">
                  <Switch
                    checked={state.isPublic}
                    onChange={(isPublic) => patch({ isPublic })}
                    label={
                      state.isPublic
                        ? 'Shared, so anyone can import it'
                        : 'Not shared, only you can use it'
                    }
                  />
                </div>
              </div>
            )}

            {isBare && (
              <p className="ed-fresh">
                Nothing set yet, so this row would show the most popular{' '}
                {state.type === 'movie' ? 'movies' : 'series'} overall. Narrow it down below.
              </p>
            )}

            {sections.map((section) => (
              <div key={section.key}>
                <button
                  type="button"
                  id={`sec-head-${section.key}`}
                  className="sec-head"
                  aria-expanded={openSection === section.key}
                  aria-controls={`sec-body-${section.key}`}
                  onClick={() =>
                    setOpenSection((current) => (current === section.key ? null : section.key))
                  }
                >
                  <span className="cr-role type-eyebrow">{section.role}</span>
                  <span className="sec-sum">{section.summary}</span>
                  <Icon icon={ChevronDown} size={16} className="ico" />
                </button>
                <div
                  id={`sec-body-${section.key}`}
                  className="sec-body"
                  hidden={openSection !== section.key}
                >
                  {section.body}
                </div>
              </div>
            ))}

            <div className="cr">
              <span className="cr-role type-eyebrow">Shuffle the results</span>
              <div className="cr-val ed-line">
                <Segmented<'off' | 'on'>
                  ariaLabel="Shuffle the results"
                  value={state.params.randomized ? 'on' : 'off'}
                  onChange={(value) => patchParams({ randomized: value === 'on' || undefined })}
                  options={[
                    { value: 'off', label: 'Off' },
                    { value: 'on', label: 'On' },
                  ]}
                />
                <span className="ed-note">{sumShuffle(Boolean(state.params.randomized))}</span>
              </div>
            </div>

            {onDelete && (
              <div className="ed-delete">
                <button type="button" className="btn-danger-text" onClick={onDelete}>
                  Delete this catalog
                </button>
                <p>{baseline.isPublic ? 'It goes for everyone who imported it.' : "It isn't shared, so only you lose it."}</p>
              </div>
            )}
          </div>

          <RecipePreview preview={preview} type={state.type} invalid={recipeInvalid} onRun={runPreview} />
        </div>
      </div>

      <SaveError noun="catalog" message={serverError} />
    </EditorShell>
  )
}

type SectionKey = 'order' | 'genres' | 'ratings' | 'lang' | 'date' | 'age' | 'watch'

/** Which section a server-mirrored validation error belongs to, so an
 *  invalid Save can open it. `name` isn't here — it's the one error outside
 *  every section, on the basics above them. */
const ERROR_SECTION: Partial<Record<string, SectionKey>> = {
  sort_by: 'order',
  vote_average: 'ratings',
  with_runtime: 'ratings',
  vote_count: 'ratings',
  certification_country: 'age',
  watch_region: 'watch',
  within_days: 'date',
}

/** The order Save focuses errors in when more than one applies at once. */
const ERROR_PRIORITY = [
  'name',
  'sort_by',
  'watch_region',
  'certification_country',
  'within_days',
  'vote_average',
  'with_runtime',
  'vote_count',
]

function roleLabelFor(key: string, isMovie: boolean): string {
  if (key === 'name') return 'Name'
  if (key === 'within_days') return isMovie ? 'Release date' : 'First aired'
  const section = ERROR_SECTION[key]
  return section ? SECTION_ROLE[section](isMovie) : key
}

const SECTION_ROLE: Record<SectionKey, (isMovie: boolean) => string> = {
  order: () => 'Order the row',
  genres: () => 'Genres',
  ratings: () => 'Ratings and runtime',
  lang: () => 'Original language',
  date: (isMovie) => (isMovie ? 'Release date' : 'First aired'),
  age: () => 'Age rating',
  watch: () => 'Where to watch',
}

/** Builds the seven collapsible sections: each carries the plain-English
 *  summary its closed head shows, so the whole recipe reads down the page
 *  without opening anything. Kept as one function rather than inlined JSX so
 *  the summaries — which need almost every piece of derived state the editor
 *  already computed — don't have to be threaded through seven separate
 *  components a second time. */
function buildSections(args: {
  state: CatalogFormState
  sortField: string
  sortDirection: 'asc' | 'desc'
  activeGenres: Genre[]
  withGenres: { ids: number[]; join: 'and' | 'or' }
  withoutGenres: { ids: number[]; join: 'and' | 'or' }
  genresExpanded: boolean
  setGenresExpanded: (expanded: boolean) => void
  languageOptions: { value: string; label: string }[]
  languages: Language[]
  isMovie: boolean
  dateGte: string | undefined
  dateLte: string | undefined
  dateDays: number | undefined
  activeCertifications: CertificationsByCountry
  countryNames: CountryLookup
  activeScale: CertificationsByCountry[string]
  watchProviderCount: number
  errorFor: (key: string) => string | undefined
  patch: (update: Partial<CatalogFormState>) => void
  patchParams: (update: Partial<TMDBParams>) => void
}) {
  const {
    state,
    sortField,
    sortDirection,
    activeGenres,
    withGenres,
    withoutGenres,
    genresExpanded,
    setGenresExpanded,
    languageOptions,
    languages,
    isMovie,
    dateGte,
    dateLte,
    dateDays,
    activeCertifications,
    countryNames,
    activeScale,
    watchProviderCount,
    errorFor,
    patch,
    patchParams,
  } = args

  return [
    {
      key: 'order' as const,
      role: 'Order the row',
      summary: sumOrder(state.type, sortField, sortDirection),
      body: (
        <>
          <div className="choices" role="group" aria-label="Order by">
            {SORT_FIELDS[state.type].map((option) => (
              <button
                key={option.value}
                type="button"
                className="choice"
                aria-pressed={sortField === option.value || (!sortField && option.value === 'popularity')}
                onClick={() =>
                  patchParams({ sort_by: serializeSortBy(option.value, sortDirection) })
                }
              >
                {option.label}
              </button>
            ))}
          </div>
          <Segmented
            ariaLabel="Direction"
            value={sortDirection}
            onChange={(direction) =>
              patchParams({ sort_by: serializeSortBy(sortField || 'popularity', direction) })
            }
            options={[
              { value: 'desc', label: 'High to low' },
              { value: 'asc', label: 'Low to high' },
            ]}
          />
          <p className="ed-note">
            This is the order of titles inside the row, not where the row sits on your home
            screen.
          </p>
          {errorFor('sort_by') && <FieldNote tone="danger">{errorFor('sort_by')}</FieldNote>}
        </>
      ),
    },
    {
      key: 'genres' as const,
      role: 'Genres',
      summary: sumGenres(activeGenres, withGenres.ids, withGenres.join, withoutGenres.ids),
      body: (
        <GenreCycler
          genres={activeGenres}
          withIds={withGenres.ids}
          withJoin={withGenres.join}
          withoutIds={withoutGenres.ids}
          expanded={genresExpanded}
          onExpandedChange={setGenresExpanded}
          onChange={(withIds, withJoin, withoutIds) =>
            patchParams({
              with_genres: withIds.length ? serializeGenreList(withIds, withJoin) : undefined,
              without_genres: withoutIds.length ? serializeGenreList(withoutIds, 'and') : undefined,
            })
          }
        />
      ),
    },
    {
      key: 'ratings' as const,
      role: 'Ratings and runtime',
      summary: sumRatings(
        state.params.vote_average_gte,
        state.params.vote_average_lte,
        state.params.vote_count_gte,
        state.params.vote_count_lte,
        state.params.with_runtime_gte,
        state.params.with_runtime_lte,
      ),
      body: (
        <>
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
        </>
      ),
    },
    {
      key: 'lang' as const,
      role: 'Original language',
      summary: sumLanguage(state.params.with_original_language, languages),
      body: (
        <>
          <Select
            value={state.params.with_original_language ?? ''}
            onChange={(value) => patchParams({ with_original_language: value || undefined })}
            placeholder="Any language"
            options={languageOptions}
          />
          {languageOptions.length === 0 && <FieldNote>Couldn't load languages.</FieldNote>}
        </>
      ),
    },
    {
      key: 'date' as const,
      role: isMovie ? 'Release date' : 'First aired',
      summary: sumDate(state.type, state.dateMode, dateGte, dateLte, dateDays),
      body: (
        <DateWindow
          state={state}
          error={errorFor('within_days')}
          onMode={(dateMode) => patch({ dateMode })}
          onParams={patchParams}
        />
      ),
    },
    {
      key: 'age' as const,
      role: 'Age rating',
      summary: sumAge(
        state.params.certification_country,
        countryNames,
        state.params.certification_gte ?? state.params.certification,
        state.params.certification_lte ?? state.params.certification,
        activeScale,
      ),
      body: (
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
      ),
    },
    {
      key: 'watch' as const,
      role: 'Where to watch',
      summary: sumWatch(state.params.watch_region, countryNames, watchProviderCount),
      body: (
        <WatchProviderPicker
          type={state.type}
          params={state.params}
          error={errorFor('watch_region')}
          onParams={patchParams}
        />
      ),
    },
  ]
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
  const gte = isMovie ? state.params.primary_release_date_gte : state.params.first_air_date_gte
  const lte = isMovie ? state.params.primary_release_date_lte : state.params.first_air_date_lte
  const days = isMovie ? state.params.released_within_days : state.params.aired_within_days

  return (
    <>
      <Segmented
        ariaLabel="Date window"
        value={state.dateMode}
        onChange={onMode}
        options={[
          { value: 'any', label: 'Any time' },
          { value: 'fixed', label: 'Fixed dates' },
          { value: 'rolling', label: 'Recent' },
        ]}
      />

      {state.dateMode === 'any' && (
        <p className="ed-note">
          No date limit. Choose Recent for a window that moves with today, like "the last 90
          days".
        </p>
      )}

      {state.dateMode === 'fixed' && (
        <div className="ed-line">
          <label className="cr-role type-eyebrow" htmlFor="ed-from">
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
          <label className="cr-role type-eyebrow" htmlFor="ed-to">
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
    </>
  )
}

/**
 * The windows people actually ask for, as a row of preset choice buttons
 * (`DATE_PRESETS`, `summary.ts`) rather than a box that wants a number of
 * days — "90" is not a thing anyone thinks in. Each preset is still just
 * `_within_days` on the wire; only the way the number is arrived at differs.
 * "Upcoming" is the same rolling filter closed up to yesterday: `_within_days`
 * becomes a `.gte` and nothing else, so a one-day window is "dated yesterday
 * or later" — over a discover page sorted by popularity, the unreleased
 * slate. It recalculates daily like every other preset, unlike a fixed `gte`
 * pinned to the day the catalog was saved, which would read as "upcoming" for
 * one day and then quietly rot.
 */
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
        <button type="button" className="choice" aria-pressed={isUpcoming} onClick={() => onDays(UPCOMING_DAYS)}>
          Upcoming
        </button>
        {oddDays !== undefined && (
          <button type="button" className="choice" aria-pressed onClick={() => onDays(oddDays)}>
            {oddDays} days
          </button>
        )}
      </div>

      <div className="ed-line">
        <label className="cr-role type-eyebrow" htmlFor="ed-years">
          Last
        </label>
        <input
          id="ed-years"
          type="number"
          min={1}
          max={50}
          value={customYears ?? ''}
          placeholder="#"
          onChange={(event) => {
            const years = Number(event.target.value)
            onDays(event.target.value === '' || years < 1 ? undefined : years * DAYS_PER_YEAR)
          }}
          className="field type-data"
        />
        <span>years</span>
      </div>

      {error && <FieldNote tone="danger">{error}</FieldNote>}

      {/* The date the chip resolves to today. The window is recomputed
          server-side per request, so this moves with the calendar — which is
          the one thing the chip's own label can't say. */}
      {days !== undefined && !error && (
        <p className="ed-dateline">
          {isUpcoming
            ? `${isMovie ? 'Released' : 'Airing'} from ${formatWindowStart(UPCOMING_DAYS)} onward, updated daily.`
            : `${isMovie ? 'Released' : 'Aired'} since ${formatWindowStart(days)}, updated daily.`}
        </p>
      )}
    </>
  )
}
