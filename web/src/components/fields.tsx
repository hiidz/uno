import type { ReactNode } from 'react'
import { Slider } from 'radix-ui'

/**
 * The form primitives shared by both builders. Nothing here is
 * catalog-specific; the catalog-shaped ones (`NumberInput`, `RangeField`,
 * `GenrePicker`) live in `features/catalogs/fields.tsx`, which also re-exports
 * these so the catalog builder has a single import site.
 */

export function Field({
  label,
  hint,
  error,
  children,
}: {
  label: string
  hint?: string
  error?: string
  children: ReactNode
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <label className="type-eyebrow">{label}</label>
      {children}
      {hint && !error && <p className="type-data text-dimmer m-0 text-[10.5px]">{hint}</p>}
      {error && <p className="type-data text-danger m-0 text-[10.5px]">{error}</p>}
    </div>
  )
}

export function TextInput({
  value,
  onChange,
  placeholder,
  type = 'text',
  invalid,
  maxLength,
  ariaLabel,
}: {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  type?: string
  invalid?: boolean
  maxLength?: number
  /** For inputs inside a row that carries its own label elsewhere — a folder
   *  title next to its controls, say — where a `<label>` would be redundant. */
  ariaLabel?: string
}) {
  return (
    <input
      type={type}
      value={value}
      placeholder={placeholder}
      maxLength={maxLength}
      aria-label={ariaLabel}
      aria-invalid={invalid || undefined}
      onChange={(event) => onChange(event.target.value)}
      className={`field w-full ${invalid ? 'border-danger' : ''}`}
    />
  )
}

export function Select({
  value,
  onChange,
  options,
  placeholder = 'Any',
}: {
  value: string
  onChange: (value: string) => void
  options: { value: string; label: string }[]
  placeholder?: string
}) {
  return (
    <select
      value={value}
      onChange={(event) => onChange(event.target.value)}
      className="field w-full"
    >
      <option value="">{placeholder}</option>
      {options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  )
}

export function Segmented<T extends string>({
  value,
  onChange,
  options,
  ariaLabel,
}: {
  value: T
  onChange: (value: T) => void
  options: { value: T; label: string }[]
  ariaLabel: string
}) {
  return (
    <div role="group" aria-label={ariaLabel} className="border-line-hi flex overflow-hidden rounded-[2px] border">
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          onClick={() => onChange(option.value)}
          aria-pressed={value === option.value}
          className={`border-line-hi hover:text-ink flex-1 border-r px-2.5 py-[6px] text-[10.5px] tracking-[0.08em] uppercase transition-colors last:border-r-0 ${
            value === option.value ? 'bg-raised-hi text-ink' : 'text-dim'
          }`}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}

/**
 * A two-thumb slider over `[min, max]`. `low`/`high` are `undefined` at the
 * respective extreme — a thumb parked at the min or max edge means "no bound
 * here", matching how the rest of the form models an unset filter, so the
 * wire payload doesn't gain a redundant `gte: 0` just because the slider has
 * to render *some* position for "unset".
 */
export function DualRangeSlider({
  min,
  max,
  step = 1,
  low,
  high,
  onLow,
  onHigh,
  formatValue = (value) => String(value),
}: {
  min: number
  max: number
  step?: number
  low: number | undefined
  high: number | undefined
  onLow: (value: number | undefined) => void
  onHigh: (value: number | undefined) => void
  formatValue?: (value: number) => string
}) {
  const lowValue = low ?? min
  const highValue = high ?? max

  return (
    <div className="flex flex-col gap-1.5">
      <Slider.Root
        className="relative flex h-4 w-full touch-none items-center select-none"
        min={min}
        max={max}
        step={step}
        value={[lowValue, highValue]}
        minStepsBetweenThumbs={0}
        onValueChange={([nextLow, nextHigh]) => {
          onLow(nextLow <= min ? undefined : nextLow)
          onHigh(nextHigh >= max ? undefined : nextHigh)
        }}
      >
        <Slider.Track className="bg-line-hi relative h-[3px] grow rounded-full">
          <Slider.Range className="bg-dim absolute h-full rounded-full" />
        </Slider.Track>
        <Slider.Thumb
          aria-label="Minimum"
          className="border-dim bg-raised-hi hover:border-ink focus-visible:ring-ink block h-[13px] w-[13px] rounded-full border shadow-sm outline-none focus-visible:ring-2"
        />
        <Slider.Thumb
          aria-label="Maximum"
          className="border-dim bg-raised-hi hover:border-ink focus-visible:ring-ink block h-[13px] w-[13px] rounded-full border shadow-sm outline-none focus-visible:ring-2"
        />
      </Slider.Root>
      <div className="type-data text-dimmer flex justify-between text-[10.5px]">
        <span>{formatValue(lowValue)}</span>
        <span>{formatValue(highValue)}</span>
      </div>
    </div>
  )
}

export function Checkbox({
  checked,
  onChange,
  label,
  hint,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  label: string
  hint?: string
}) {
  return (
    <label className="flex cursor-pointer items-start gap-2.5">
      <input
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
        className="accent-movie mt-[3px] h-[13px] w-[13px]"
      />
      <span className="flex flex-col gap-0.5">
        <span className="text-[12.5px]">{label}</span>
        {hint && <span className="type-data text-dimmer text-[10.5px]">{hint}</span>}
      </span>
    </label>
  )
}
