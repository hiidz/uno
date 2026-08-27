import { useEffect, useMemo, useState } from 'react'
import type { CertificationsByCountry, Genre, TMDBParams } from '@/api'
import { EditorShell } from '@/features/builder/EditorShell'
import { useRecipeTiles } from '@/features/preview/useRecipeTiles'
import {
  LANGUAGES,
} from './languages'
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
  CertificationPicker,
  Checkbox,
  Field,
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
  saving,
  serverError,
  onSave,
  onRequestClose,
  onDirtyChange,
}: {
  mode: BuilderMode
  initial: CatalogFormState | null
  genres: { movie: Genre[]; tv: Genre[] }
  certifications: { movie: CertificationsByCountry; tv: CertificationsByCountry }
  saving: boolean
  /** Plain-text body of a server 400. Should be unreachable — the form mirrors
   *  every rule — so it renders as an unexpected-case banner, not a field. */
  serverError: string | null
  onSave: (state: CatalogFormState) => void
  onRequestClose: () => void
  onDirtyChange: (dirty: boolean) => void
}) {
  const baseline = useMemo(() => initial ?? emptyForm(), [initial])
  const [state, setState] = useState<CatalogFormState>(baseline)
  const [showErrors, setShowErrors] = useState(false)

  // Fed the recipe as it stands on every render, but only *fetches* when the
  // preview block's button is pressed — see `useRecipeTiles`. `name` and
  // `is_public` aren't part of a recipe, so renaming a catalog doesn't make
  // its preview stale.
  const preview = useRecipeTiles(state.type, paramsString(state))
  const resetPreview = preview.reset

  useEffect(() => {
    setState(baseline)
    setShowErrors(false)
    // Seeding a different catalog means the tiles on screen belong to the
    // previous one. Stale is the wrong word for that — they aren't this
    // recipe's results at all — so they go rather than being labelled.
    resetPreview()
  }, [baseline, resetPreview])

  const dirty = !isSameCatalog(baseline, state)
  useEffect(() => onDirtyChange(dirty), [dirty, onDirtyChange])

  const errors = useMemo(() => validateForm(state), [state])
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

  function submit() {
    setShowErrors(true)
    if (Object.keys(errors).length > 0) return
    onSave(state)
  }

  /** Same shape as `submit`: reveal what's wrong, or go. Errors stay hidden
   *  until something is submitted, so pressing Preview has to be one of the
   *  things that reveals them — otherwise the note explaining why it won't run
   *  points at highlighting that isn't there yet. */
  function runPreview() {
    if (recipeInvalid) {
      setShowErrors(true)
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
      footer={
        <>
          {showErrors && Object.keys(errors).length > 0 && (
            <span className="type-data text-danger mr-auto text-[10.5px]">
              Fix the highlighted {Object.keys(errors).length === 1 ? 'field' : 'fields'}.
            </span>
          )}
          <button type="button" onClick={onRequestClose} className="btn-ghost">
            Cancel
          </button>
          <button type="button" onClick={submit} disabled={saving} className="btn-primary">
            {saving ? 'Saving…' : mode === 'edit' ? 'Save changes' : 'Create catalog'}
          </button>
        </>
      }
    >
        <div className="grid gap-x-8 gap-y-5 md:grid-cols-[minmax(0,240px)_minmax(0,1fr)]">
          {/* --- identity ------------------------------------------------ */}
          <div className="flex flex-col gap-5">
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
              hint={
                mode === 'edit'
                  ? "Can't be changed. Duplicate this catalog to make a series version."
                  : 'Choose which type to see the appropriate filter options.'
              }
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
              hint="Anyone signed in can add it to their own home screen."
            />
          </div>

          {/* --- filters -------------------------------------------------- */}
          <div className="flex flex-col gap-5">
            <Field label="Sort by" error={errorFor('sort_by')}>
              <div className="flex items-center gap-2">
                <Select
                  value={sortField}
                  onChange={(field) =>
                    patchParams({ sort_by: serializeSortBy(field, sortDirection) })
                  }
                  placeholder="Popularity"
                  options={SORT_FIELDS[state.type]}
                />
                <div className="w-[104px] shrink-0">
                  <Segmented
                    ariaLabel="Sort direction"
                    value={sortDirection}
                    onChange={(direction) =>
                      patchParams({ sort_by: serializeSortBy(sortField || 'popularity', direction) })
                    }
                    options={[
                      { value: 'desc', label: 'High–low' },
                      { value: 'asc', label: 'Low–high' },
                    ]}
                  />
                </div>
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

            <div className="grid gap-x-4 gap-y-5 sm:grid-cols-2">
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
                label="Vote count"
                hint="Filters out obscure titles with a handful of ratings."
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
                  options={LANGUAGES.map(({ code, name }) => ({ value: code, label: name }))}
                />
              </Field>
            </div>

            <DateWindow
              state={state}
              error={errorFor('within_days')}
              onMode={(dateMode) => patch({ dateMode })}
              onParams={patchParams}
            />

            <CertificationPicker
              label="Certification"
              countries={activeCertifications}
              country={state.params.certification_country}
              gte={state.params.certification_gte ?? state.params.certification}
              lte={state.params.certification_lte ?? state.params.certification}
              error={errorFor('certification_country')}
              onChange={(update) => patchParams({ ...update, certification: undefined })}
            />

            <WatchProviders
              params={state.params}
              error={errorFor('watch_region')}
              onParams={patchParams}
            />

            <Checkbox
              checked={Boolean(state.params.randomized)}
              onChange={(randomized) => patchParams({ randomized: randomized || undefined })}
              label="Shuffle results"
              hint="Randomizes the order each time someone opens the row. Pulls from a random page of matches."
            />
          </div>
        </div>

        <RecipePreview preview={preview} invalid={recipeInvalid} onRun={runPreview} />

        {serverError && (
          <p className="type-data text-danger border-danger mt-5 border-l-2 pl-3 text-[11px]">
            The server rejected this catalog: {serverError}
          </p>
        )}
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
      <div className="flex items-center gap-3">
        <label className="type-eyebrow flex-1">{label}</label>
        <div className="w-[210px]">
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
            className="field type-data w-full"
          />
          <span className="text-dimmer shrink-0 text-[12px]">–</span>
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
            className="field type-data w-full"
          />
        </div>
      )}

      {state.dateMode === 'rolling' && (
        <div className="flex flex-col gap-1.5">
          <div className="flex items-center gap-2">
            <input
              type="number"
              min={1}
              value={days ?? ''}
              placeholder="90"
              onChange={(event) => {
                const value = event.target.value === '' ? undefined : Number(event.target.value)
                onParams(
                  isMovie ? { released_within_days: value } : { aired_within_days: value },
                )
              }}
              className="field type-data w-[110px]"
            />
            <span className="type-data text-dimmer text-[11px]">
              {isMovie ? 'days since release' : 'days since an episode aired'}
            </span>
          </div>
          {error && <p className="type-data text-danger m-0 text-[10.5px]">{error}</p>}
          <p className="type-data text-dimmer m-0 text-[10.5px]">
            Updates each time someone opens the row, so it always shows the most recent content.
          </p>
        </div>
      )}
    </div>
  )
}


/**
 * Watch providers and region, likewise one control. There is no provider-list
 * endpoint, so ids are free text — the hint carries the only guidance
 * available.
 */
function WatchProviders({
  params,
  error,
  onParams,
}: {
  params: TMDBParams
  error?: string
  onParams: (update: Partial<TMDBParams>) => void
}) {
  return (
    <Field
      label="Streaming on"
      hint="Streaming service IDs, comma-separated (e.g., 8, 9, 337). Find IDs in the TMDB documentation."
      error={error}
    >
      <div className="flex items-center gap-2">
        <input
          value={params.with_watch_providers ?? ''}
          onChange={(event) =>
            onParams({
              with_watch_providers: event.target.value.trim() || undefined,
              watch_region: event.target.value.trim()
                ? (params.watch_region ?? 'US')
                : undefined,
            })
          }
          placeholder="any"
          className="field type-data w-full"
        />
        {params.with_watch_providers && (
          <>
            <span className="type-data text-dimmer shrink-0 text-[11px]">in</span>
            <input
              value={params.watch_region ?? ''}
              onChange={(event) =>
                onParams({ watch_region: event.target.value.toUpperCase() || undefined })
              }
              placeholder="US"
              maxLength={2}
              className="field type-data w-[70px]"
            />
          </>
        )}
      </div>
    </Field>
  )
}
