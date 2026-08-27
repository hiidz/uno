import type { Certification, CertificationsByCountry, Genre } from '@/api'
import { DualRangeSlider, Field, Segmented, Select } from '@/components/fields'
import type { GenreJoin } from './catalogForm'

/**
 * Form primitives for the catalog builder.
 *
 * The "required together" pairs are single components rather than two fields
 * plus an assertion, and the date window is one mode toggle rather than four
 * independent inputs. The server's validation rules can't be reported per-field
 * (its 400s are plain text), so the form encodes them structurally: an invalid
 * combination is unrepresentable rather than merely caught.
 *
 * The generic primitives (`Field`, `TextInput`, `Select`, `Segmented`,
 * `Checkbox`) live in `components/fields.tsx`, shared with the collection
 * builder, and are re-exported here so this stays the catalog builder's one
 * import site.
 */

export { Checkbox, Field, Segmented, Select, TextInput } from '@/components/fields'

/** A number field that models "unset" as undefined rather than 0 — Go's
 *  `omitempty` drops zeros, so 0 and absent are the same on the wire, and
 *  treating an empty box as 0 would silently add a filter. */
export function NumberInput({
  value,
  onChange,
  placeholder,
  step,
  min,
}: {
  value: number | undefined
  onChange: (value: number | undefined) => void
  placeholder?: string
  step?: string
  min?: number
}) {
  return (
    <input
      type="number"
      inputMode="decimal"
      step={step}
      min={min}
      value={value ?? ''}
      placeholder={placeholder}
      onChange={(event) => {
        const raw = event.target.value
        onChange(raw === '' ? undefined : Number(raw))
      }}
      className="field type-data w-full"
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
    <Field label={label} hint={hint} error={error}>
      <div className="flex flex-col gap-2.5">
        <DualRangeSlider
          min={min}
          max={max}
          step={step ? Number(step) : 1}
          low={low}
          high={high}
          onLow={onLow}
          onHigh={onHigh}
          formatValue={formatValue}
        />
        <div className="flex items-center gap-2">
          <NumberInput value={low} onChange={onLow} placeholder="min" step={step} min={min} />
          <span className="text-dimmer shrink-0 text-[12px]">–</span>
          <NumberInput value={high} onChange={onHigh} placeholder="max" step={step} min={min} />
          {unit && <span className="type-data text-dimmer shrink-0 text-[11px]">{unit}</span>}
        </div>
      </div>
    </Field>
  )
}

/**
 * Genre picker, one row. Each chip cycles neutral → include → exclude → neutral
 * on click, rather than living in two separate pickers — a genre picked in
 * both "genres" and "exclude genres" used to be representable and made no
 * sense (TMDB would just cancel it out), so the cycle makes that state
 * unreachable instead of merely confusing.
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
      <div className="flex items-center gap-3">
        <label className="type-eyebrow flex-1">{label}</label>
        {withIds.length > 1 && (
          <div className="w-[132px]">
            <Segmented
              ariaLabel={`How to combine ${label.toLowerCase()}`}
              value={withJoin}
              onChange={(next) => onChange(withIds, next, withoutIds)}
              options={[
                { value: 'and', label: 'All of' },
                { value: 'or', label: 'Any of' },
              ]}
            />
          </div>
        )}
      </div>
      {genres.length === 0 ? (
        <p className="type-data text-dimmer m-0 text-[10.5px]">
          Couldn't load genres from TMDB.
        </p>
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
                    ? `Excluding ${genre.name} — click to clear`
                    : state === 'include'
                      ? `Including ${genre.name} — click to exclude instead`
                      : `Click to include ${genre.name}`
                }
                className={`rounded-[2px] border px-2 py-[3px] text-[11px] transition-colors ${
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
 * thing, in another country's.
 */
export function CertificationPicker({
  label,
  countries,
  country,
  gte,
  lte,
  error,
  onChange,
}: {
  label: string
  countries: CertificationsByCountry
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
  const codes = Object.keys(countries).sort((a, b) =>
    a === 'US' ? -1 : b === 'US' ? 1 : a.localeCompare(b),
  )
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
      <label className="type-eyebrow">{label}</label>
      <div className="flex items-center gap-3">
        <div className="w-[90px] shrink-0">
          <Select
            value={country ?? ''}
            onChange={(next) =>
              onChange({
                certification_country: next || undefined,
                certification_gte: undefined,
                certification_lte: undefined,
              })
            }
            placeholder="Country"
            options={codes.map((code) => ({ value: code, label: code }))}
          />
        </div>
        {country && scale.length > 0 && (
          <div className="flex-1">
            <DualRangeSlider
              min={0}
              max={lastIndex}
              step={1}
              low={lowIndex}
              high={highIndex}
              onLow={(next) => onSlide(next, highIndex)}
              onHigh={(next) => onSlide(lowIndex, next)}
              formatValue={(index) => scale[index]?.certification ?? '–'}
            />
          </div>
        )}
      </div>
      {country && scale.length === 0 && (
        <p className="type-data text-dimmer m-0 text-[10.5px]">
          Couldn't load ratings for {country} from TMDB.
        </p>
      )}
      {error && <p className="type-data text-danger m-0 text-[10.5px]">{error}</p>}
    </div>
  )
}
