import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronDown } from 'lucide-react'
import { fetchCollection, queryKeys } from '@/api'
import type { CertificationsByCountry, Genre, Language, TMDBParams } from '@/api'
import { Icon } from '@/components/Icon'
import type { SignStep } from '@/components/PaneSign'
import { EditorFooter } from '@/features/builder/EditorFooter'
import { EditorShell } from '@/features/builder/EditorShell'
import { useEditorForm } from '@/features/builder/useEditorForm'
import { buildGenreLookup, recipeSentence, typeLabel } from '@/features/library/recipe'
import { useRecipeTiles } from '@/features/preview/useRecipeTiles'
import type { CountryLookup } from './countries'
import {
  SORT_FIELDS,
  isCollectionRow,
  isSameCatalog,
  paramsString,
  parseSortBy,
  serializeSortBy,
  validateForm,
  type FieldErrors,
  type CatalogFormState,
  type SourceMode,
} from './catalogForm'
import {
  parseIdList,
  serializeIdList,
  type IdJoin,
} from './params'
import { EntityLists, NameSetting, ScopeSetting, ShuffleControl, SourceModeSetting, Talker } from './CatalogSettings'
import { DateWindow } from './DateWindow'
import { RecipePreview } from './RecipePreview'
import {
  sumAge,
  sumCollection,
  sumDate,
  sumEntities,
  sumGenres,
  sumLanguage,
  sumOrder,
  sumRatings,
  sumWatch,
  withShuffle,
} from './summary'
import { TMDBEntityPicker } from './TMDBEntityPicker'
import { WatchProviderPicker } from './WatchProviderPicker'
import {
  CertificationPicker,
  FieldNote,
  GenreCycler,
  RangeField,
  Segmented,
  Select,
} from './fields'

/** What the sections are handed to read errors and change the form. */
type ErrorFor = (key: string) => string | undefined
type FormPatch = Partial<CatalogFormState>
type PatchForm = (update: FormPatch) => void
type ParamsPatch = Partial<TMDBParams>
type PatchParams = (update: ParamsPatch) => void

/** The stored genre lists for a genre pick: included and left out, each
 *  only while it holds an id. */
function genreParams(withIds: number[], withJoin: IdJoin, withoutIds: number[]): Partial<TMDBParams> {
  return {
    with_genres: withIds.length ? serializeIdList(withIds, withJoin) : undefined,
    without_genres: withoutIds.length ? serializeIdList(withoutIds, 'and') : undefined,
  }
}

/** A range end at the slider's top reads as open-ended. */
function formatVotes(value: number): string {
  return value >= 5000 ? '5000+' : String(value)
}

function formatRuntime(value: number): string {
  return value >= 300 ? '300+' : String(value)
}

/** Which half of the per-type lookups a catalog type reads: genres and
 *  certification scales are keyed `movie` and `tv`. */
const TYPE_KEY = { movie: 'movie', series: 'tv' } as const

/** This type's own date fields: the release date for a movie, the first air
 *  date for a series. */
const DATE_KEYS = {
  movie: ['primary_release_date_gte', 'primary_release_date_lte', 'released_within_days'],
  series: ['first_air_date_gte', 'first_air_date_lte', 'aired_within_days'],
} as const

/** What the form's type decides: which age-rating scales apply,
 *  which date fields it reads, and the picked film series. */
function typeFacts(state: CatalogFormState, certifications: CatalogEditorProps['certifications']) {
  const activeCertifications = certifications[TYPE_KEY[state.type]]
  const [gteKey, lteKey, daysKey] = DATE_KEYS[state.type]
  return {
    isMovie: state.type === 'movie',
    // The age-rating scales differ between movie and tv, though both are
    // scoped by the same certification_country.
    activeCertifications,
    activeScale: scaleOf(activeCertifications, state.params.certification_country),
    dateGte: state.params[gteKey],
    dateLte: state.params[lteKey],
    dateDays: state.params[daysKey],
    // The picked TMDB collection (not the Uno collection `state.collectionID`
    // scopes this catalog to).
    tmdbCollectionID: filmSeriesID(state),
  }
}

function scaleOf(certifications: CertificationsByCountry, country: string | undefined) {
  return (country && certifications[country]) || []
}

function filmSeriesID(state: CatalogFormState): number | undefined {
  if (state.type !== 'movie') return undefined
  return parseIdList(state.params.with_collection).ids[0]
}

/** The picked film series' name, through the same by-id key the picker's chip
 *  reads, so the section head names it without a request of its own. */
function useFilmSeriesName(id: number | undefined): string | undefined {
  const seriesID = id ?? 0
  const query = useQuery({
    queryKey: queryKeys.collection(seriesID),
    queryFn: () => fetchCollection(seriesID),
    enabled: id !== undefined,
    staleTime: Infinity,
  })
  return query.data?.name
}

/** Where the errors on show are, for the save bar's status line; none until
 *  errors are on show. */
function errorRoleLabels(errors: FieldErrors, isMovie: boolean, showErrors: boolean): string[] {
  if (!showErrors) return []
  return Array.from(new Set(Object.keys(errors).map((key) => roleLabelFor(key, isMovie))))
}

/** The editor's name, or what an unnamed one is called. */
function catalogTitle(name: string): string {
  return name.trim() || 'Untitled catalog'
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

/** The sections a preview waits on, named once each: every error but the
 *  name, which is no part of a recipe. */
function recipeSections(errors: FieldErrors, isMovie: boolean): string[] {
  const names = new Set<string>()
  for (const key of Object.keys(errors)) {
    if (key !== 'name') names.add(roleLabelFor(key, isMovie))
  }
  return [...names]
}

function roleLabelFor(key: string, isMovie: boolean): string {
  if (key === 'name') return 'Name'
  if (key === 'within_days') return isMovie ? 'Release date' : 'First aired'
  const section = ERROR_SECTION[key]
  return section ? SECTION_ROLE[section](isMovie) : key
}

const SECTION_ROLE: Record<SectionKey, (isMovie: boolean) => string> = {
  order: () => 'Order',
  genres: () => 'Genres',
  ratings: () => 'Ratings and runtime',
  lang: () => 'Original language',
  date: (isMovie) => (isMovie ? 'Release date' : 'First aired'),
  age: () => 'Age rating',
  watch: () => 'Where to watch',
  companies: () => 'Studios',
  keywords: () => 'Keywords',
  networks: () => 'Networks',
  collection: () => 'Film series',
}

/** Builds the collapsible sections: each carries the plain-English
 *  summary its closed head shows, so the whole recipe reads down the page
 *  without opening anything. Kept as one function rather than inlined JSX so
 *  the summaries — which need almost every piece of derived state the editor
 *  already computed — don't have to be threaded through a component per
 *  section a second time. Each section's title is `SECTION_ROLE`'s. */
interface SectionArgs {
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
  errorFor: ErrorFor
  patch: PatchForm
  patchParams: PatchParams
}

function buildSections(args: SectionArgs) {
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

  // The handlers the section bodies hand their controls.
  const setDirection = (direction: 'asc' | 'desc') =>
    patchParams({ sort_by: serializeSortBy(sortField || 'popularity', direction) })
  const setGenres = (withIds: number[], withJoin: IdJoin, withoutIds: number[]) =>
    patchParams(genreParams(withIds, withJoin, withoutIds))
  const setLanguage = (value: string) => patchParams({ with_original_language: value || undefined })

  const sections = [
    {
      key: 'order' as const,
      summary: withShuffle(sumOrder(state.type, sortField, sortDirection), state.params.randomized),
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
            onChange={setDirection}
            options={[
              { value: 'desc', label: 'High to low' },
              { value: 'asc', label: 'Low to high' },
            ]}
          />
          {errorFor('sort_by') && <FieldNote tone="danger">{errorFor('sort_by')}</FieldNote>}
          <ShuffleControl randomized={state.params.randomized} filmSeries={false} onParams={patchParams} />
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
          onChange={setGenres}
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
            formatValue={formatVotes}
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
            formatValue={formatRuntime}
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
            onChange={setLanguage}
            placeholder="Any language"
            options={languageOptions}
          />
          {languageOptions.length === 0 && <FieldNote>Couldn’t load languages.</FieldNote>}
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
            body: <>
                <TMDBEntityPicker
                  kind="network"
                  type={state.type}
                  value={state.params.with_networks}
                  onChange={(with_networks) => patchParams({ with_networks })}
                />
                {errorFor('with_networks') && (
                  <FieldNote tone="danger">{errorFor('with_networks')}</FieldNote>
                )}
              </>,
          },
        ]),
    // Movie only: TMDB has no collections for series.
    ...(isMovie
      ? [
          {
            key: 'collection' as const,
            summary: withShuffle(sumCollection(state.params.with_collection, tmdbCollectionName), state.params.randomized),
            body: <>
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
                <ShuffleControl randomized={state.params.randomized} filmSeries onParams={patchParams} />
              </>,
          },
        ]
      : []),
  ]

  // Film series mode shows the film series section alone and filters mode
  // everything else, matching what `recipeParams` in catalogForm.ts sends.
  const collectionRow = isCollectionRow(state)
  return sections.filter((section) => (section.key === 'collection') === collectionRow)
}


interface CatalogEditorProps {
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
  /** "Done" in a collection's nested editor, whose save only stages the edit
   *  for the collection's own Save. */
  saveLabel?: string
  /** A listed catalog's next step with Community, as the sign's button.
   *  Absent in a collection's nested editor, where a scoped catalog shows its
   *  Scope. */
  sharingStep?: SignStep
  /** Its sharing stickers, beside the kind on the sign. */
  sharingBadges?: ReactNode
}

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
 * **Every row this editor opens is the profile's own and editable.** A catalog
 * added from Community opens as a view instead (`FromCommunityView`). Its next
 * step with Community is `sharingStep`, which the pane builds and the sign
 * carries as its one button.
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
  saveLabel = 'Save',
  sharingStep,
  sharingBadges,
}: CatalogEditorProps) {
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
  // preview block's button is pressed — see `useRecipeTiles`. `name` isn't
  // part of a recipe, so renaming a catalog doesn't make its preview stale.
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

  const { isMovie, activeCertifications, activeScale, dateGte, dateLte, dateDays, tmdbCollectionID } =
    typeFacts(state, certifications)
  // Genre ids differ between movie and tv, so the list follows `type`.
  const activeGenres = genres[TYPE_KEY[state.type]]
  const genreLookup = useMemo(() => buildGenreLookup(activeGenres), [activeGenres])
  const recipeWords = useMemo(
    () => recipeSentence({ type: state.type, params }, genreLookup),
    [state.type, params, genreLookup],
  )
  const withGenres = parseIdList(state.params.with_genres)
  const withoutGenres = parseIdList(state.params.without_genres)

  const { field: sortField, direction: sortDirection } = parseSortBy(state.params.sort_by)

  const watchProviderCount = parseIdList(state.params.with_watch_providers).ids.length
  const tmdbCollectionName = useFilmSeriesName(tmdbCollectionID)

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

  function trySubmit() {
    if (errorCount > 0) {
      revealErrors()
      focusFirstError()
      return
    }
    submit(errorCount, (finalState) => onSave(finalState))
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

  const roleLabels = errorRoleLabels(errors, isMovie, showErrors)

  return (
    <EditorShell
      purpose="Edit catalog"
      tone="catalog"
      badges={
        <>
          <span className="stk stk-neutral">{typeLabel(state.type)}</span>
          {sharingBadges}
        </>
      }
      step={sharingStep}
      title={catalogTitle(state.name)}
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
          saveLabel={saveLabel}
          saveError={serverError}
        />
      }
    >
      <div className="ed-container">
        <div className="ed ed-results">
          <div className="ed-form">
            <Talker words={recipeWords} />
            <NameSetting value={state.name} error={errorFor('name')} onChange={patch} />

            <ScopeSetting
              scoped={state.collectionID !== null}
              canMoveToLibrary={canMoveToLibrary}
              onChange={patch}
            />

            <div className="setting is-head">
              <h2 className="setting-label m-0">What the row shows</h2>
            </div>

            <SourceModeSetting isMovie={isMovie} value={state.sourceMode} onChange={switchSourceMode} />

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
          </div>

          <RecipePreview
            preview={preview}
            type={state.type}
            invalid={recipeInvalid}
            fixFirst={recipeSections(errors, isMovie)}
            onRun={runPreview}
          />
        </div>
      </div>
    </EditorShell>
  )
}
