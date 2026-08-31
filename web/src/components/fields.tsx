import type { ReactNode } from 'react'
import { Slider, Tooltip } from 'radix-ui'

/**
 * The form primitives shared by both builders. Nothing here is
 * catalog-specific; the catalog-shaped ones (`NumberInput`, `RangeField`,
 * `GenreCycler`, `CertificationPicker`) live in `features/catalogs/fields.tsx`,
 * which also re-exports these so the catalog builder has a single import site.
 *
 * **Each control's width is a property of what it holds**, expressed against
 * the `--w-*` tokens in `index.css` rather than filling whatever it is given.
 * A pane that is as wide as the window stretched every one of these to it, and
 * a select over "Grid" and "List" set 700px wide is the clearest way a form
 * can say nobody looked at it.
 */

export function Field({
  label,
  hint,
  tip,
  error,
  children,
}: {
  label: string
  /** A line under the control. For something the user has to know *before*
   *  filling it in — the rest belongs in `tip`. */
  hint?: string
  /** Secondary explanation, behind an icon beside the label. See `InfoTip`. */
  tip?: string
  error?: string
  children: ReactNode
}) {
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-1.5">
        <label className="type-eyebrow">{label}</label>
        {tip && <InfoTip label={label} text={tip} />}
      </div>
      {children}
      {hint && !error && <FieldNote>{hint}</FieldNote>}
      {error && <FieldNote tone="danger">{error}</FieldNote>}
    </div>
  )
}

/**
 * A field's secondary explanation, behind an icon.
 *
 * **Not a place to move hints to.** A hint that survives "does the user need
 * this before they can fill the field in" stays on the page as a `FieldNote`;
 * one that doesn't survive "is this needed at all" is deleted. This is for the
 * narrow middle: a control whose behaviour is genuinely worth a sentence, in a
 * dense grid where that sentence would push every neighbouring field down a
 * line.
 *
 * Its own `Tooltip.Provider` rather than one at the app root — there are a
 * handful of these, they never appear side by side, and a root provider would
 * be a shared setting nothing else uses.
 */
export function InfoTip({ label, text }: { label: string; text: string }) {
  return (
    <Tooltip.Provider delayDuration={120}>
      <Tooltip.Root>
        <Tooltip.Trigger asChild>
          <button
            type="button"
            aria-label={`About ${label.toLowerCase()}`}
            className="border-line-hi text-dimmer hover:border-dim hover:text-dim grid h-[13px] w-[13px] shrink-0 place-items-center rounded-full border text-[8px] leading-none transition-colors"
          >
            <span aria-hidden="true">?</span>
          </button>
        </Tooltip.Trigger>
        <Tooltip.Portal>
          <Tooltip.Content
            side="top"
            align="start"
            sideOffset={5}
            collisionPadding={12}
            className="type-data bg-raised-hi border-line-hi text-dim z-50 max-w-[15rem] rounded-[2px] border px-2.5 py-1.5 text-[10.5px] leading-[1.45]"
          >
            {text}
          </Tooltip.Content>
        </Tooltip.Portal>
      </Tooltip.Root>
    </Tooltip.Provider>
  )
}

/**
 * The line under a control: its hint, or what's wrong with it. One component so
 * the two never disagree about size or leading — they occupy the same slot and
 * swap, and a half-pixel difference between them shows as a jump.
 */
export function FieldNote({
  children,
  tone = 'dim',
}: {
  children: ReactNode
  tone?: 'dim' | 'danger'
}) {
  return (
    <p
      className={`type-data m-0 max-w-[var(--w-entry)] text-[11px] leading-[1.45] ${
        tone === 'danger' ? 'text-danger' : 'text-dimmer'
      }`}
    >
      {children}
    </p>
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
  width,
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
  /** How much room the content actually needs. Omitted for the genuinely
   *  unbounded — a URL, a search — which fill what they're given. */
  width?: string
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
      style={width ? { maxWidth: width } : undefined}
      className={`field w-full ${invalid ? 'border-danger' : ''}`}
    />
  )
}

/**
 * A select over a fixed list.
 *
 * **`placeholder` is what decides whether empty is even reachable.** Omitted,
 * there is no empty option at all: the options *are* the values, which is what
 * a closed enum like a view mode needs — one used to gain a fourth entry from
 * whatever the placeholder said, a value the type didn't have.
 *
 * Given, `clearable` then decides what empty means. Off, the empty option is a
 * choice in its own right — "Any language" is a filter setting, and picking it
 * back is how you undo a language. On, the empty option is a placeholder:
 * `disabled hidden` keeps it out of the open list, the closed control renders
 * it dimmed, and an X beside the chevron is the only way back to it. That last
 * one is for where "no country" isn't one of the countries — an age rating
 * scale belongs to a country or to nothing at all.
 */
export function Select({
  value,
  onChange,
  options,
  placeholder,
  clearable = false,
  clearLabel = 'selection',
  /** How wide the widest option needs the closed control to be. Defaults to
   *  the pick width; a select over country codes asks for less. */
  width = 'var(--w-pick)',
}: {
  value: string
  onChange: (value: string) => void
  options: { value: string; label: string }[]
  placeholder?: string
  /** Empty is a placeholder rather than a choice: unreachable from the list,
   *  reached instead through the X this adds. */
  clearable?: boolean
  /** Names what the X clears, for anyone who can't see which field it sits in. */
  clearLabel?: string
  width?: string
}) {
  const select = (
    <select
      value={value}
      onChange={(event) => onChange(event.target.value)}
      style={clearable ? undefined : { maxWidth: width }}
      className={`field w-full ${clearable ? 'field--clearable' : ''} ${
        clearable && !value ? 'text-dimmer' : ''
      }`}
    >
      {placeholder != null && (
        <option value="" disabled={clearable} hidden={clearable}>
          {placeholder}
        </option>
      )}
      {options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  )

  if (!clearable) return select

  // The width moves to the wrapper so the X lands against the control's own
  // edge; left on the select, the button would float at the end of whatever
  // room the row happened to give it.
  return (
    <div className="relative w-full" style={{ maxWidth: width }}>
      {select}
      {value && (
        <button
          type="button"
          aria-label={`Clear ${clearLabel}`}
          onClick={() => onChange('')}
          className="text-dimmer hover:text-ink absolute top-1/2 right-[27px] grid h-4 w-4 -translate-y-1/2 place-items-center text-[13px] leading-none transition-colors"
        >
          <span aria-hidden="true">&times;</span>
        </button>
      )}
    </div>
  )
}

/**
 * A row of mutually exclusive choices, sized by its labels.
 *
 * **It sizes itself.** Every caller used to wrap it in a fixed pixel width
 * guessed from the labels inside it, and the guesses drifted: "High–low"
 * wrapped onto two lines, and "Square" was clipped to "Sq". `w-fit` plus
 * `whitespace-nowrap` makes the wrapper unnecessary and the clipping
 * unreachable — a label that grows takes the room it needs instead of losing
 * its tail.
 */
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
    <div
      role="group"
      aria-label={ariaLabel}
      className="border-line-hi flex w-fit shrink-0 overflow-hidden rounded-[2px] border"
    >
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          onClick={() => onChange(option.value)}
          aria-pressed={value === option.value}
          className={`border-line-hi hover:text-ink flex-1 border-r px-3 py-1.5 text-[10.5px] tracking-[0.08em] whitespace-nowrap uppercase transition-colors last:border-r-0 ${
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
 *
 * **One `onChange` carrying both bounds, not an `onLow` and an `onHigh`.** A
 * drag on either thumb produces both values at once, and reporting them
 * through two callbacks meant a consumer that writes the pair as one object
 * ran the second write over the first — `CertificationPicker` did, and the
 * effect was that its low thumb could not be moved at all: the second call
 * put the old low back from a stale closure. A control that moves two values
 * together has to say so in its signature.
 */
export function DualRangeSlider({
  min,
  max,
  step = 1,
  low,
  high,
  onChange,
  formatValue = (value) => String(value),
  showValues = true,
}: {
  min: number
  max: number
  step?: number
  low: number | undefined
  high: number | undefined
  onChange: (low: number | undefined, high: number | undefined) => void
  formatValue?: (value: number) => string
  /** Off where the caller already prints the pair — the number boxes beside a
   *  `RangeField`'s label say the same thing, and twice is once too many. */
  showValues?: boolean
}) {
  const lowValue = low ?? min
  const highValue = high ?? max

  return (
    <div className="flex max-w-[var(--w-track)] flex-col gap-1.5">
      <Slider.Root
        className="relative flex h-4 w-full touch-none items-center select-none"
        min={min}
        max={max}
        step={step}
        value={[lowValue, highValue]}
        minStepsBetweenThumbs={0}
        onValueChange={([nextLow, nextHigh]) =>
          onChange(nextLow <= min ? undefined : nextLow, nextHigh >= max ? undefined : nextHigh)
        }
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
      {/* Each value sits under its own thumb rather than at the track's end.
          Pinned to the ends they read as axis labels — "0" under the far left
          of a runtime track whose low thumb is parked there is right by
          coincidence, and "140" at the far right, where the track means 300,
          is simply wrong. */}
      {showValues && (
        <div className="relative h-[11px]">
          <ThumbValue fraction={fractionOf(lowValue, min, max)} label={formatValue(lowValue)} />
          <ThumbValue fraction={fractionOf(highValue, min, max)} label={formatValue(highValue)} />
        </div>
      )}
    </div>
  )
}

function fractionOf(value: number, min: number, max: number): number {
  if (max === min) return 0
  return (value - min) / (max - min)
}

/**
 * One value, centred on its thumb. The thumb's own centre travels between
 * half a thumb-width in from each end, which is what `calc` reproduces — a
 * plain percentage would drift a few pixels wide at the extremes, exactly
 * where the two labels are furthest apart and the offset is most visible.
 */
function ThumbValue({ fraction, label }: { fraction: number; label: string }) {
  return (
    <span
      style={{ left: `calc(${fraction * 100}% + ${(0.5 - fraction) * 13}px)` }}
      className="type-data text-dimmer absolute top-0 -translate-x-1/2 text-[10.5px] leading-none whitespace-nowrap"
    >
      {label}
    </span>
  )
}

/**
 * A checkbox with its label and, optionally, a line under it.
 *
 * **Disabled greys it without changing what it holds.** A setting that doesn't
 * apply under the current shape of the form is still a setting the user chose,
 * and clearing it on their behalf would send a payload they never asked for —
 * so the box keeps its value and the `hint` says why it can't be touched. A
 * greyed box with no explanation is the version people file bugs about.
 */
export function Checkbox({
  checked,
  onChange,
  label,
  hint,
  disabled = false,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  label: string
  hint?: string
  disabled?: boolean
}) {
  return (
    <label
      className={`group flex max-w-[var(--w-entry)] items-start gap-2.5 ${
        disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'
      }`}
    >
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
        className="checkbox mt-[3px]"
      />
      <span className="flex flex-col gap-1">
        <span
          className={`text-[12.5px] transition-colors ${disabled ? '' : 'group-hover:text-ink'}`}
        >
          {label}
        </span>
        {hint && (
          <span className="type-data text-dimmer text-[11px] leading-[1.45]">{hint}</span>
        )}
      </span>
    </label>
  )
}
