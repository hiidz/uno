import type { Certification, CertificationsByCountry, Genre } from '@/api'
import { DualRangeSlider, FieldNote, InfoTip, Segmented, Select } from '@/components/fields'
import type { GenreJoin } from './catalogForm'
import { countryName, type CountryLookup } from './countries'

/**
 * Form primitives for the catalog builder.
 *
 * The "required together" pairs are single components rather than two fields
 * plus an assertion, and the date window is one mode toggle rather than four
 * independent inputs. The server's validation rules can't be reported per-field
 * (its 400s are plain text), so the form encodes them structurally: an invalid
 * combination is unrepresentable rather than merely caught.
 *
 * The generic primitives (`Field`, `FieldNote`, `TextInput`, `Select`,
 * `Segmented`, `Checkbox`) live in `components/fields.tsx`, shared with the
 * collection builder, and are re-exported here so this stays the catalog
 * builder's one import site.
 */

export {
  Checkbox,
  Field,
  FieldNote,
  InfoTip,
  Segmented,
  Select,
  TextInput,
} from '@/components/fields'

/** A number field that models "unset" as undefined rather than 0 — Go's
 *  `omitempty` drops zeros, so 0 and absent are the same on the wire, and
 *  treating an empty box as 0 would silently add a filter. */
export function NumberInput({
  value,
  onChange,
  placeholder,
  step,
  min,
  ariaLabel,
}: {
  value: number | undefined
  onChange: (value: number | undefined) => void
  placeholder?: string
  step?: string
  min?: number
  /** These sit on a row whose only text is the range's own label, so each box
   *  has to name which end of it it holds. */
  ariaLabel?: string
}) {
  return (
    <input
      type="number"
      inputMode="decimal"
      step={step}
      min={min}
      value={value ?? ''}
      placeholder={placeholder}
      aria-label={ariaLabel}
      onChange={(event) => {
        const raw = event.target.value
        onChange(raw === '' ? undefined : Number(raw))
      }}
      className="field type-data w-full max-w-[var(--w-code)] min-w-0 px-2 py-1.5 text-center text-[12px] pointer-coarse:text-[16px]"
    />
  )
}

/**
 * A slider spanning `[min, max]`, backed by the same two-point range as the
 * server's `_gte`/`_lte` pair. The slider covers the range almost everyone
 * needs; the number boxes below it stay for the rare exact value (or one
 * past the slider's ceiling) that dragging can't reach precisely.
 */
export function RangeField({
  label,
  hint,
  error,
  unit,
  low,
  high,
  onLow,
  onHigh,
  step,
  min,
  max,
  formatValue,
}: {
  label: string
  hint?: string
  error?: string
  unit?: string
  low: number | undefined
  high: number | undefined
  onLow: (value: number | undefined) => void
  onHigh: (value: number | undefined) => void
  step?: string
  min: number
  max: number
  formatValue?: (value: number) => string
}) {
  return (
    <div className="flex max-w-[var(--w-track)] flex-col gap-2">
      {/* Label and bounds on one line, track underneath spanning the full
          width. The boxes are the exact values the track can only approximate,
          so they belong beside the name of the thing they bound rather than
          stacked below it as a second, wider control. */}
      <div className="flex items-center gap-3">
        <label className="type-eyebrow flex-1">
          {label}
          {unit && <span className="text-dimmer normal-case"> ({unit})</span>}
        </label>
        <NumberInput
          value={low}
          onChange={(next) => onLow(next)}
          ariaLabel={`Minimum ${label.toLowerCase()}`}
          placeholder={String(min)}
          step={step}
          min={min}
        />
        <span aria-hidden="true" className="text-dimmer shrink-0 text-[12.5px]">
          —
        </span>
        <NumberInput
          value={high}
          onChange={(next) => onHigh(next)}
          ariaLabel={`Maximum ${label.toLowerCase()}`}
          placeholder={formatValue ? formatValue(max) : String(max)}
          step={step}
          min={min}
        />
      </div>

      {/* `showValues` off: the boxes above already print this pair, and a
          second copy travelling under the thumbs was the same number twice. */}
      <DualRangeSlider
        min={min}
        max={max}
        step={step ? Number(step) : 1}
        low={low}
        high={high}
        onChange={(nextLow, nextHigh) => {
          onLow(nextLow)
          onHigh(nextHigh)
        }}
        formatValue={formatValue}
        showValues={false}
      />

      {hint && !error && <FieldNote>{hint}</FieldNote>}
      {error && <FieldNote tone="danger">{error}</FieldNote>}
    </div>
  )
}

/**
 * Genre picker, one row. Each chip cycles neutral → include → exclude → neutral
 * on click, rather than living in two separate pickers — a genre picked in
 * both "genres" and "exclude genres" would be representable and make no sense
 * (TMDB would just cancel it out), so the cycle makes that state unreachable
 * instead of merely confusing.
 */
export function GenreCycler({
  label,
  genres,
  withIds,
  withJoin,
  withoutIds,
  onChange,
}: {
  label: string
  genres: Genre[]
  withIds: number[]
  withJoin: GenreJoin
  withoutIds: number[]
  onChange: (withIds: number[], withJoin: GenreJoin, withoutIds: number[]) => void
}) {
  function cycle(id: number) {
    if (withIds.includes(id)) {
      // include -> exclude
      onChange(
        withIds.filter((g) => g !== id),
        withJoin,
        [...withoutIds, id],
      )
    } else if (withoutIds.includes(id)) {
      // exclude -> neutral
      onChange(withIds, withJoin, withoutIds.filter((g) => g !== id))
    } else {
      // neutral -> include
      onChange([...withIds, id], withJoin, withoutIds)
    }
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-3">
        <label className="type-eyebrow">{label}</label>
        {withIds.length > 1 && (
          <Segmented
            ariaLabel={`How to combine ${label.toLowerCase()}`}
            value={withJoin}
            onChange={(next) => onChange(withIds, next, withoutIds)}
            options={[
              { value: 'and', label: 'All of' },
              { value: 'or', label: 'Any of' },
            ]}
          />
        )}
      </div>
      {genres.length === 0 ? (
        <FieldNote>Couldn't load genres.</FieldNote>
      ) : (
        <div className="flex flex-wrap gap-1.5">
          {genres.map((genre) => {
            const state = withIds.includes(genre.id)
              ? 'include'
              : withoutIds.includes(genre.id)
                ? 'exclude'
                : 'neutral'
            return (
              <button
                key={genre.id}
                type="button"
                onClick={() => cycle(genre.id)}
                aria-pressed={state !== 'neutral'}
                title={
                  state === 'exclude'
                    ? `No ${genre.name} — click to stop filtering on it`
                    : state === 'include'
                      ? `Only ${genre.name} — click to exclude it instead`
                      : `Click to require ${genre.name}`
                }
                className={`rounded-[2px] border px-2 py-1 text-[11px] transition-colors pointer-coarse:py-2 ${
                  state === 'include'
                    ? 'bg-raised-hi border-dim text-ink'
                    : state === 'exclude'
                      ? 'border-danger text-danger bg-transparent line-through decoration-1'
                      : 'border-line text-dim hover:border-dim hover:text-ink'
                }`}
              >
                {genre.name}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}

/**
 * Country + min/max age rating, as one control. TMDB's `certification` filter
 * only ever compares against one country's scale — a rating means nothing
 * without knowing whose system it is — so the range is scoped to the chosen
 * country, not a free multiselect.
 *
 * The range is a slider over the scale's *position*, not its codes — codes
 * like "PG-13" aren't orderable on their own, only via each entry's `order`
 * from TMDB. Dragging the low thumb above the high one just carries the high
 * thumb along (`DualRangeSlider`'s `minStepsBetweenThumbs={0}` still lets them
 * meet), so "M18 below G" is unrepresentable instead of merely invalid.
 *
 * Switching country clears both bounds: a rating code from one country's
 * scale (say, US's "R") doesn't necessarily exist, and never means the same
 * thing, in another country's. The country itself is a country or nothing —
 * there is no "any country" scale to compare a rating against — so the empty
 * value is the select's placeholder, reached back through its X.
 */
export function CertificationPicker({
  label,
  tip,
  countries,
  countryNames,
  country,
  gte,
  lte,
  error,
  onChange,
}: {
  label: string
  /** Secondary explanation, behind an icon beside the label. */
  tip?: string
  countries: CertificationsByCountry
  /** TMDB's certification response is keyed by code with no name attached —
   *  this is what resolves each key to something a person reads. */
  countryNames: CountryLookup
  country: string | undefined
  gte: string | undefined
  lte: string | undefined
  error?: string
  onChange: (update: {
    certification_country: string | undefined
    certification_gte: string | undefined
    certification_lte: string | undefined
  }) => void
}) {
  // Sorted by the name shown, not by the code behind it: listed by code, "GB"
  // sits between "FR" and "HU" while reading "United Kingdom", and the list
  // looks unsorted. US stays pinned first — it is the scale most of these
  // catalogs are built against.
  const codes = Object.keys(countries).sort((a, b) => {
    if (a === 'US') return -1
    if (b === 'US') return 1
    return countryName(a, countryNames).localeCompare(countryName(b, countryNames))
  })
  const scale: Certification[] = country
    ? [...(countries[country] ?? [])].sort((a, b) => a.order - b.order)
    : []

  const indexOf = (code: string | undefined) => {
    const index = code ? scale.findIndex((entry) => entry.certification === code) : -1
    return index === -1 ? undefined : index
  }
  const lowIndex = indexOf(gte)
  const highIndex = indexOf(lte)
  const lastIndex = Math.max(scale.length - 1, 0)

  function onSlide(nextLowIndex: number | undefined, nextHighIndex: number | undefined) {
    onChange({
      certification_country: country,
      certification_gte: nextLowIndex != null ? scale[nextLowIndex]?.certification : undefined,
      certification_lte: nextHighIndex != null ? scale[nextHighIndex]?.certification : undefined,
    })
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-1.5">
        <label className="type-eyebrow">{label}</label>
        {tip && <InfoTip label={label} text={tip} />}
      </div>
      {/* `items-start`, not `items-center`: the slider carries a value caption
          under it and the select doesn't, so centring the two hangs the select
          half a line low. */}
      <div className="flex items-start gap-3">
        <Select
          value={country ?? ''}
          onChange={(next) =>
            onChange({
              certification_country: next || undefined,
              certification_gte: undefined,
              certification_lte: undefined,
            })
          }
          placeholder="Select a country"
          clearable
          clearLabel="age rating country"
          options={codes.map((code) => ({ value: code, label: countryName(code, countryNames) }))}
        />
        {country && scale.length > 0 && (
          <div className="min-w-0 flex-1 pt-1.5">
            {/* One `onChange`, both bounds: two separate calls would each
                carry only one bound from a stale closure, and whichever ran
                last would overwrite the other with it. */}
            <DualRangeSlider
              min={0}
              max={lastIndex}
              step={1}
              low={lowIndex}
              high={highIndex}
              onChange={onSlide}
              formatValue={(index) => scale[index]?.certification ?? '–'}
            />
          </div>
        )}
      </div>
      {country && scale.length === 0 && (
        <FieldNote>Couldn't load ratings for {countryName(country, countryNames)}.</FieldNote>
      )}
      {error && <FieldNote tone="danger">{error}</FieldNote>}
    </div>
  )
}
