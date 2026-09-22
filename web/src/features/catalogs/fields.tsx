import { Check, X } from 'lucide-react'
import type { Certification, CertificationsByCountry, Genre } from '@/api'
import { DualRangeSlider, FieldNote, Segmented, Select } from '@/components/fields'
import { Icon } from '@/components/Icon'
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
 * The generic primitives the catalog builder uses (`FieldNote`, `TextInput`,
 * `Select`, `Segmented`, `Switch`) live in `components/fields.tsx`, shared with
 * the collection builder, and are re-exported here so this stays the catalog
 * builder's one import site.
 */

export { FieldNote, Segmented, Select, Switch, TextInput } from '@/components/fields'

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
      className="field type-data h-8 w-full max-w-[var(--w-code)] min-w-0 px-2 text-center text-[12px] pointer-coarse:h-11 pointer-coarse:text-[16px]"
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
    // `w-full`: sized to the track rather than to its label, so every range in
    // a section draws the same track length and its boxes end on one edge.
    <div className="flex w-full max-w-[var(--w-track)] flex-col gap-2">
      {/* Label and bounds on one line, track underneath spanning the full
          width. The boxes are the exact values the track can only approximate,
          so they belong beside the name of the thing they bound rather than
          stacked below it as a second, wider control. */}
      <div className="flex items-center gap-2">
        <label className="type-eyebrow mr-1 min-w-0 flex-1 leading-[1.3]">
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
        label={label.toLowerCase()}
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
 *
 * Every genre shows at once: TMDB's list for either type is about twenty
 * short names, a few rows of chips. The all/any join only appears once two
 * genres are included, because with fewer there is nothing for it to combine.
 */
export function GenreCycler({
  genres,
  withIds,
  withJoin,
  withoutIds,
  onChange,
}: {
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

  if (genres.length === 0) return <FieldNote>Couldn't load genres.</FieldNote>

  return (
    <>
      <p className="ed-note">Press once to include, twice to leave out, again to ignore.</p>
      <div className="choices" role="group" aria-label="Genres">
        {genres.map((genre) => {
          const state = withIds.includes(genre.id)
            ? 'inc'
            : withoutIds.includes(genre.id)
              ? 'exc'
              : 'off'
          const next = { off: 'include', inc: 'leave out', exc: 'ignore' }[state]
          return (
            <button
              key={genre.id}
              type="button"
              className="gchip"
              data-g={state}
              onClick={() => cycle(genre.id)}
              aria-label={`${genre.name}: ${
                { off: 'ignored', inc: 'included', exc: 'left out' }[state]
              }. Press to ${next}.`}
            >
              {state === 'inc' && <Icon icon={Check} size={14} />}
              {state === 'exc' && <Icon icon={X} size={14} />}
              <span className="gname">{genre.name}</span>
            </button>
          )
        })}
      </div>
      {withIds.length >= 2 && (
        <div className="ed-line">
          <span className="cr-role type-eyebrow">Included genres</span>
          <Segmented
            ariaLabel="How to combine included genres"
            value={withJoin}
            onChange={(next) => onChange(withIds, next, withoutIds)}
            options={[
              { value: 'and', label: 'All of them' },
              { value: 'or', label: 'Any of them' },
            ]}
          />
        </div>
      )}
    </>
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
  countries,
  countryNames,
  country,
  gte,
  lte,
  error,
  onChange,
}: {
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
    // No label of its own: the section head above already says "Age rating".
    <div className="flex w-full flex-col gap-2">
      {/* Stacked, not side by side: the select takes `--w-pick`, and on a
          narrow value column a slider beside it is left a few dozen pixels —
          thumbs on top of each other and captions overlapping. Under the
          select it gets the same `--w-track` as every other range. */}
      <div className="flex flex-col gap-3">
        <Select
          ariaLabel="Age rating country"
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
          <div className="w-full max-w-[var(--w-track)]">
            {/* One `onChange`, both bounds: two separate calls would each
                carry only one bound from a stale closure, and whichever ran
                last would overwrite the other with it. */}
            <DualRangeSlider
              label="age rating"
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
      {!country && <FieldNote>Ratings differ by country. Pick one to set a range.</FieldNote>}
      {country && scale.length === 0 && (
        <FieldNote>Couldn't load ratings for {countryName(country, countryNames)}.</FieldNote>
      )}
      {error && <FieldNote tone="danger">{error}</FieldNote>}
    </div>
  )
}
