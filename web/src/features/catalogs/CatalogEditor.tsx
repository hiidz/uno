import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, Tag } from 'lucide-react'
import { fetchCollection, queryKeys } from '@/api'
import type { CertificationsByCountry, Genre, Language, TMDBParams } from '@/api'
import { Icon } from '@/components/Icon'
import { EditorFooter } from '@/features/builder/EditorFooter'
import { EditorShell } from '@/features/builder/EditorShell'
import { ConfirmUnlink, LinkedBanner } from '@/features/builder/LinkedCopy'
import { useEditorForm } from '@/features/builder/useEditorForm'
import { buildGenreLookup, recipeSentence, typeLabel } from '@/features/library/recipe'
import { useRecipeTiles } from '@/features/preview/useRecipeTiles'
import type { CountryLookup } from './countries'
import {
  SORT_FIELDS,
  changesContent,
  isCollectionRow,
  isSameCatalog,
  paramsString,
  parseSortBy,
  serializeSortBy,
  validateForm,
  type CatalogFormState,
  type DateMode,
  type SourceMode,
} from './catalogForm'
import {
  DATE_PRESETS,
  DAYS_PER_YEAR,
  UPCOMING_DAYS,
  parseIdList,
  serializeIdList,
} from './params'
import { RecipePreview } from './RecipePreview'
import {
  formatWindowStart,
  sumAge,
  sumCollection,
  sumDate,
  sumEntities,
  sumGenres,
  sumLanguage,
  sumOrder,
  sumRatings,
  sumShuffle,
  sumWatch,
} from './summary'
import { TMDBEntityPicker } from './TMDBEntityPicker'
import { WatchProviderPicker } from './WatchProviderPicker'
import {
  CertificationPicker,
  FieldError,
  FieldNote,
  GenreCycler,
  RangeField,
  Segmented,
  Select,
  Switch,
  TextInput,
} from './fields'

/**
 * Edit a catalog, filling the builder's right pane — or, for a catalog scoped
 * to a collection, a modal over that collection's editor. Always a saved row
 * (or a collection's staged draft): a catalog is named into existence by its
 * own dialog before this editor opens, and Duplicate is one server call
 * (`Workspace.tsx`'s `confirmDuplicateCatalog`) whose finished copy opens here
 * like any other row.
 *
 * **Dirtiness is reported, not handled.** Every way out of this editor
 * originates outside it — the × in the shell, Escape, selecting another row in
 * the rail, switching profile — so the pane owns the confirmation and this only
 * has to say whether there is anything to lose.
 *
 * **There is no read-only "imported" view.** The closed-graph sharing model
 * has no such state: a taken catalog is a private copy you fully own from
 * the moment it's created, not a live pointer that could ever need a
 * read-only screen. Every row this editor opens is yours. While a taken
 * copy is still `linked`, a banner says so, and a save that would unlink it
 * asks first.
 *
 * **`type` is always locked.** It is chosen when the catalog is named, and
 * nothing — here or anywhere else — changes an existing row's type.
 */
export function CatalogEditor({
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
  canMoveToLibrary = true,
  linked = false,
}: {
  /** The form as the row stands. A new identity re-seeds the editor (see
   *  `useEditorForm`), so callers hand over a stable object. */
  initial: CatalogFormState
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
   *  in a collection's nested editor, and until the library lists a row that
   *  was just created. */
  onDuplicate?: () => void
  onDelete?: () => void
  onDirtyChange: (dirty: boolean) => void
  /** False for a catalog staged inside a collection that isn't a row yet: it
   *  is created scoped when the collection saves, so there is nothing to move
   *  until then. */
  canMoveToLibrary?: boolean
  /** A library catalog still linked to the community catalog it was taken
   *  from: shows the linked banner, and a save that changes anything besides
   *  Public asks first (`ConfirmUnlink`). */
  linked?: boolean
}) {
  const baseline = initial
  const { state, setState, dirty, showErrors, revealErrors, submit } = useEditorForm(
    baseline,
    isSameCatalog,
    onDirtyChange,
  )

  // One section open at a time: each folds open in place under its own head.
  // Every body stays mounted regardless — toggled with
  // `hidden`, not unmounted — because `WatchProviderPicker` and
  // `TMDBEntityPicker` keep their own region/search state locally and losing
  // it every time the section closes would mean re-picking a region on every
  // reopen.
  // A collection row has one section, so it starts open.
  const [openSection, setOpenSection] = useState<SectionKey | null>(() =>
    isCollectionRow(baseline) ? 'collection' : null,
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
  const params = paramsString(state)
  const preview = useRecipeTiles(state.type, params)
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

  // Genre ids differ between movie and tv, so the list follows `type`.
  const activeGenres = state.type === 'movie' ? genres.movie : genres.tv
  const genreLookup = useMemo(() => buildGenreLookup(activeGenres), [activeGenres])
  const recipeWords = useMemo(
    () => recipeSentence({ type: state.type, params }, genreLookup),
    [state.type, params, genreLookup],
  )
  const withGenres = parseIdList(state.params.with_genres)
  const withoutGenres = parseIdList(state.params.without_genres)

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
  const watchProviderCount = parseIdList(state.params.with_watch_providers).ids.length

  // The picked TMDB collection (not the Uno collection `state.collectionID`
  // scopes this catalog to), through the same by-id key the picker's chip
  // reads, so the section head names it without a request of its own.
  const tmdbCollectionID = isMovie ? parseIdList(state.params.with_collection).ids[0] : undefined
  const tmdbCollectionName = useQuery({
    queryKey: queryKeys.collection(tmdbCollectionID ?? 0),
    queryFn: () => fetchCollection(tmdbCollectionID ?? 0),
    enabled: tmdbCollectionID !== undefined,
    staleTime: Infinity,
  }).data?.name

  function patch(update: Partial<CatalogFormState>) {
    setState((previous) => ({ ...previous, ...update }))
  }

  function patchParams(update: Partial<TMDBParams>) {
    setState((previous) => ({ ...previous, params: { ...previous.params, ...update } }))
  }

  function switchSourceMode(sourceMode: SourceMode) {
    patch({ sourceMode })
    setOpenSection(sourceMode === 'collection' ? 'collection' : null)
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
    const target = section === 'collection' ? 'cat-collection' : `sec-head-${section}`
    requestAnimationFrame(() => document.getElementById(target)?.focus())
  }

  const [confirmingUnlink, setConfirmingUnlink] = useState(false)

  function trySubmit() {
    if (errorCount > 0) {
      revealErrors()
      focusFirstError()
      return
    }
    submit(errorCount, (finalState) => {
      if (linked && changesContent(baseline, finalState)) setConfirmingUnlink(true)
      else onSave(finalState)
    })
  }

  function confirmUnlink() {
    setConfirmingUnlink(false)
    onSave(state)
  }

  const sections = buildSections({
    state,
    sortField,
    sortDirection,
    activeGenres,
    withGenres,
    withoutGenres,
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
    tmdbCollectionName,
    errorFor,
    patch,
    patchParams,
  })

  const roleLabels = showErrors
    ? Array.from(
        new Set(Object.keys(errors).map((key) => roleLabelFor(key, isMovie))),
      )
    : []

  return (
    <EditorShell
      purpose="Edit catalog"
      tone="catalog"
      badges={
        <>
          <span className="stk">{typeLabel(state.type)}</span>
          {state.isPublic && <span className="stk stk-shared">Shared</span>}
        </>
      }
      title={state.name.trim() || 'Untitled catalog'}
      onRequestClose={onRequestClose}
      onDuplicate={onDuplicate}
      onDelete={onDelete}
      docked="results"
      footer={
        <EditorFooter
          noun="catalog"
          saving={saving}
          errorCount={errorCount}
          errorLabels={roleLabels}
          dirty={dirty}
          onCancel={onRequestClose}
          onSubmit={trySubmit}
          saveLabel="Save"
          saveError={serverError}
        />
      }
    >
      <div className="ed-container">
        <div className="ed ed-results">
          <div className="ed-form">
            {/* What this row puts on the TV, in the same words the rail and the
                home screen use, kept current as the settings below change. */}
            <p className="talker">
              <Icon icon={Tag} size={20} />
              <span>{recipeWords || 'No filters yet. Set what this row shows below.'}</span>
            </p>
            {linked && <LinkedBanner noun="catalog" />}
            <div className="setting">
              <label htmlFor="cat-name" className="setting-label type-label">
                Name
              </label>
              <div className="setting-value">
                <TextInput
                  id="cat-name"
                  value={state.name}
                  onChange={(name) => patch({ name })}
                  placeholder="Trending Sci-Fi"
                  invalid={Boolean(errorFor('name'))}
                  width="100%"
                />
                {errorFor('name') && <FieldError>{errorFor('name')}</FieldError>}
              </div>
            </div>

            {/* Two short settings side by side. */}
            <div className="setting-pair">
              <div className="setting">
                <span className="setting-label type-label">Movies or series</span>
                <div className="setting-value">
                  <span className="type-data text-[15px]">
                    {state.type === 'movie' ? 'Movie' : 'Series'}
                  </span>
                </div>
              </div>

              {isMovie && (
                <div className="setting">
                  <span className="setting-label type-label">Mode</span>
                  <div className="setting-value">
                    <Segmented<SourceMode>
                      ariaLabel="Mode"
                      value={state.sourceMode}
                      onChange={switchSourceMode}
                      options={[
                        { value: 'filters', label: 'Filters' },
                        { value: 'collection', label: 'Collection' },
                      ]}
                    />
                  </div>
                </div>
              )}
            </div>

            {state.collectionID !== null ? (
              <div className="setting">
                <span className="setting-label type-label">Scope</span>
                <div className="setting-value ed-line">
                  <span className="type-data text-[13px]">Only inside this collection</span>
                  {canMoveToLibrary && (
                    <button
                      type="button"
                      className="btn-secondary btn-sm"
                      onClick={() => patch({ collectionID: null, isPublic: false })}
                    >
                      Move to library
                    </button>
                  )}
                </div>
                <p className="type-data text-dimmer m-0 pt-1 text-[12.5px] leading-[1.45]">
                  {canMoveToLibrary
                    ? 'Move to library to reuse or share it. Applies when you save the collection.'
                    : 'Save the collection first to move this to your library.'}
                </p>
              </div>
            ) : (
              <div className="setting">
                <span className="setting-label type-label">Sharing</span>
                <div className="setting-value">
                  <Switch
                    checked={state.isPublic}
                    onChange={(isPublic) => patch({ isPublic })}
                    label={state.isPublic ? 'Shared' : 'Private'}
                  />
                </div>
              </div>
            )}

            <div className="setting is-head">
              <h2 className="setting-label m-0">What the row shows</h2>
            </div>

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
                  <span className="setting-label type-label">{SECTION_ROLE[section.key](isMovie)}</span>
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

            <div className="setting">
              <span className="setting-label type-label">Shuffle the results</span>
              <div className="setting-value ed-line">
                <Segmented<'off' | 'on'>
                  ariaLabel="Shuffle the results"
                  value={state.params.randomized ? 'on' : 'off'}
                  onChange={(value) => patchParams({ randomized: value === 'on' || undefined })}
                  options={[
                    { value: 'off', label: 'Off' },
                    { value: 'on', label: 'On' },
                  ]}
                />
                {state.params.randomized && (
                  <span className="ed-note">{sumShuffle(isCollectionRow(state))}</span>
                )}
              </div>
            </div>
          </div>

          <RecipePreview preview={preview} type={state.type} invalid={recipeInvalid} onRun={runPreview} />
        </div>
      </div>

      <ConfirmUnlink
        open={confirmingUnlink}
        noun="catalog"
        name={state.name.trim() || 'this catalog'}
        onConfirm={confirmUnlink}
        onCancel={() => setConfirmingUnlink(false)}
      />
    </EditorShell>
  )
}

type SectionKey =
  | 'order'
  | 'genres'
  | 'ratings'
  | 'lang'
  | 'date'
  | 'age'
  | 'watch'
  | 'companies'
  | 'keywords'
  | 'networks'
  | 'collection'

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
  with_companies: 'companies',
  with_keywords: 'keywords',
  without_companies: 'companies',
  without_keywords: 'keywords',
  with_networks: 'networks',
  with_collection: 'collection',
}

/** The order Save focuses errors in when more than one applies at once. */
const ERROR_PRIORITY = [
  'name',
  'with_collection',
  'sort_by',
  'watch_region',
  'certification_country',
  'within_days',
  'vote_average',
  'with_runtime',
  'vote_count',
  'with_companies',
  'without_companies',
  'with_keywords',
  'without_keywords',
  'with_networks',
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
  companies: () => 'Production companies',
  keywords: () => 'Keywords',
  networks: () => 'Networks',
  collection: () => 'Collection',
}

/** Builds the collapsible sections: each carries the plain-English
 *  summary its closed head shows, so the whole recipe reads down the page
 *  without opening anything. Kept as one function rather than inlined JSX so
 *  the summaries — which need almost every piece of derived state the editor
 *  already computed — don't have to be threaded through a component per
 *  section a second time. Each section's title is `SECTION_ROLE`'s. */
function buildSections(args: {
  state: CatalogFormState
  sortField: string
  sortDirection: 'asc' | 'desc'
  activeGenres: Genre[]
  withGenres: ReturnType<typeof parseIdList>
  withoutGenres: ReturnType<typeof parseIdList>
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
  tmdbCollectionName: string | undefined
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
    tmdbCollectionName,
    errorFor,
    patch,
    patchParams,
  } = args

  const sections = [
    {
      key: 'order' as const,
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
          {errorFor('sort_by') && <FieldNote tone="danger">{errorFor('sort_by')}</FieldNote>}
        </>
      ),
    },
    {
      key: 'genres' as const,
      summary: sumGenres(activeGenres, withGenres.ids, withGenres.join, withoutGenres.ids),
      body: (
        <GenreCycler
          genres={activeGenres}
          withIds={withGenres.ids}
          withJoin={withGenres.join}
          withoutIds={withoutGenres.ids}
          onChange={(withIds, withJoin, withoutIds) =>
            patchParams({
              with_genres: withIds.length ? serializeIdList(withIds, withJoin) : undefined,
              without_genres: withoutIds.length ? serializeIdList(withoutIds, 'and') : undefined,
            })
          }
        />
      ),
    },
    {
      key: 'ratings' as const,
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
            onChange={(vote_average_gte, vote_average_lte) => patchParams({ vote_average_gte, vote_average_lte })}
            step="0.1"
            min={0}
            max={10}
          />
          <RangeField
            label="Number of ratings"
            error={errorFor('vote_count')}
            low={state.params.vote_count_gte}
            high={state.params.vote_count_lte}
            onChange={(vote_count_gte, vote_count_lte) => patchParams({ vote_count_gte, vote_count_lte })}
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
            onChange={(with_runtime_gte, with_runtime_lte) => patchParams({ with_runtime_gte, with_runtime_lte })}
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
      summary: sumLanguage(state.params.with_original_language, languages),
      body: (
        <>
          <Select
            ariaLabel="Original language"
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
      summary: sumDate(state.type, state.dateMode, dateGte, dateLte, dateDays),
      body: (
        <DateWindow
          isMovie={isMovie}
          mode={state.dateMode}
          gte={dateGte}
          lte={dateLte}
          days={dateDays}
          error={errorFor('within_days')}
          onMode={(dateMode) => patch({ dateMode })}
          onParams={patchParams}
        />
      ),
    },
    {
      key: 'age' as const,
      summary: sumAge(
        state.params.certification_country,
        countryNames,
        state.params.certification_gte ?? state.params.certification,
        state.params.certification_lte ?? state.params.certification,
        activeScale,
      ),
      body: (
        <CertificationPicker
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
    {
      key: 'companies' as const,
      summary: sumEntities(
        state.params.with_companies,
        state.params.without_companies,
        'studio',
        'Any studio',
      ),
      body: (
        <EntityLists
          kind="company"
          type={state.type}
          withValue={state.params.with_companies}
          withoutValue={state.params.without_companies}
          withError={errorFor('with_companies')}
          withoutError={errorFor('without_companies')}
          onWith={(with_companies) => patchParams({ with_companies })}
          onWithout={(without_companies) => patchParams({ without_companies })}
        />
      ),
    },
    {
      key: 'keywords' as const,
      summary: sumEntities(
        state.params.with_keywords,
        state.params.without_keywords,
        'keyword',
        'Any keywords',
      ),
      body: (
        <EntityLists
          kind="keyword"
          type={state.type}
          withValue={state.params.with_keywords}
          withoutValue={state.params.without_keywords}
          withError={errorFor('with_keywords')}
          withoutError={errorFor('without_keywords')}
          onWith={(with_keywords) => patchParams({ with_keywords })}
          onWithout={(without_keywords) => patchParams({ without_keywords })}
        />
      ),
    },
    // Series only: /discover/movie has no network filter.
    ...(isMovie
      ? []
      : [
          {
            key: 'networks' as const,
            summary: sumEntities(state.params.with_networks, undefined, 'network', 'Any network'),
            body: (
              <>
                <TMDBEntityPicker
                  kind="network"
                  type={state.type}
                  value={state.params.with_networks}
                  onChange={(with_networks) => patchParams({ with_networks })}
                />
                {errorFor('with_networks') && (
                  <FieldNote tone="danger">{errorFor('with_networks')}</FieldNote>
                )}
              </>
            ),
          },
        ]),
    // Movie only: TMDB has no collections for series.
    ...(isMovie
      ? [
          {
            key: 'collection' as const,
            summary: sumCollection(state.params.with_collection, tmdbCollectionName),
            body: (
              <>
                <TMDBEntityPicker
                  kind="collection"
                  type={state.type}
                  inputId="cat-collection"
                  value={state.params.with_collection}
                  onChange={(with_collection) => patchParams({ with_collection })}
                />
                {errorFor('with_collection') && (
                  <FieldNote tone="danger">{errorFor('with_collection')}</FieldNote>
                )}
                <FieldNote>The row lists the collection's films in release order.</FieldNote>
              </>
            ),
          },
        ]
      : []),
  ]

  // Collection mode shows the collection picker alone and filters mode
  // everything else, matching what `recipeParams` in catalogForm.ts sends.
  const collectionRow = isCollectionRow(state)
  return sections.filter((section) => (section.key === 'collection') === collectionRow)
}

/**
 * A company or keyword section's two lists: the ids a title must match, and
 * the ids that drop a title. Each list's search hides the other's picks, so
 * one id can't be in both.
 */
function EntityLists({
  kind,
  type,
  withValue,
  withoutValue,
  withError,
  withoutError,
  onWith,
  onWithout,
}: {
  kind: 'company' | 'keyword'
  type: CatalogFormState['type']
  withValue: string | undefined
  withoutValue: string | undefined
  withError: string | undefined
  withoutError: string | undefined
  onWith: (value: string | undefined) => void
  onWithout: (value: string | undefined) => void
}) {
  const withIds = useMemo(() => parseIdList(withValue).ids, [withValue])
  const withoutIds = useMemo(() => parseIdList(withoutValue).ids, [withoutValue])
  return (
    <>
      <div className="flex w-full flex-col gap-2">
        <label className="setting-label type-label" htmlFor={`cat-${kind}-with`}>
          Include
        </label>
        <TMDBEntityPicker
          kind={kind}
          type={type}
          inputId={`cat-${kind}-with`}
          value={withValue}
          hiddenIds={withoutIds}
          onChange={onWith}
        />
        {withError && <FieldNote tone="danger">{withError}</FieldNote>}
      </div>
      <div className="flex w-full flex-col gap-2">
        <label className="setting-label type-label" htmlFor={`cat-${kind}-without`}>
          Leave out
        </label>
        <TMDBEntityPicker
          kind={kind}
          type={type}
          inputId={`cat-${kind}-without`}
          exclude
          value={withoutValue}
          hiddenIds={withIds}
          onChange={onWithout}
        />
        {withoutError && <FieldNote tone="danger">{withoutError}</FieldNote>}
      </div>
    </>
  )
}

/**
 * Fixed range XOR rolling window, as one mode toggle. The server rejects both
 * being set; making the mode the control means that state can't be expressed.
 */
function DateWindow({
  isMovie,
  mode,
  gte,
  lte,
  days,
  error,
  onMode,
  onParams,
}: {
  isMovie: boolean
  mode: DateMode
  /** This type's own fixed bounds and rolling window: the release date's for
   *  a movie, the first air date's for a series. */
  gte: string | undefined
  lte: string | undefined
  days: number | undefined
  error?: string
  onMode: (mode: DateMode) => void
  onParams: (update: Partial<TMDBParams>) => void
}) {
  return (
    <>
      <Segmented
        ariaLabel="Date window"
        value={mode}
        onChange={onMode}
        options={[
          { value: 'any', label: 'Any time' },
          { value: 'fixed', label: 'Fixed dates' },
          { value: 'rolling', label: 'Recent' },
        ]}
      />

      {mode === 'fixed' && (
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
      )}

      {mode === 'rolling' && (
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
 * (`DATE_PRESETS`, `params.ts`) rather than a box that wants a number of
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

  // What was typed, kept while it still describes `days`. Derived from `days`
  // alone, the box could never hold "1" (a year is the chip's), so typing "10"
  // would clear itself at the first keystroke.
  const [yearsText, setYearsText] = useState('')
  const typedYears = Number(yearsText)
  const yearsValue =
    yearsText !== '' && typedYears >= 1 && typedYears * DAYS_PER_YEAR === days
      ? yearsText
      : (customYears ?? '')

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
        <label className="setting-label type-label" htmlFor="ed-years">
          Last
        </label>
        <input
          id="ed-years"
          type="number"
          min={1}
          max={50}
          value={yearsValue}
          placeholder="#"
          onChange={(event) => {
            setYearsText(event.target.value)
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
