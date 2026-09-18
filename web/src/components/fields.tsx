import type { ReactNode } from 'react'
import { Popover, Slider } from 'radix-ui'

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
 * **A popover, not a tooltip.** A Radix tooltip opens on hover and on
 * keyboard focus, and closes on pointer-down — so on a touch screen there is no
 * gesture that opens it at all, and the sentence simply isn't in the app. This
 * opens on click, which every input device has.
 */
export function InfoTip({ label, text }: { label: string; text: string }) {
  return (
    <Popover.Root>
      <Popover.Trigger asChild>
        <button
          type="button"
          aria-label={`About ${label.toLowerCase()}`}
          className="tap border-line-hi text-dimmer hover:border-dim hover:text-dim grid h-[13px] w-[13px] shrink-0 place-items-center rounded-full border text-[8px] leading-none transition-colors"
        >
          <span aria-hidden="true">?</span>
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          side="top"
          align="start"
          sideOffset={5}
          collisionPadding={12}
          className="type-data bg-raised-hi border-line-hi text-dim z-50 max-w-[15rem] rounded-[2px] border px-2.5 py-1.5 text-[10.5px] leading-[1.45]"
        >
          {text}
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
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
  id,
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
  /** Pairs this input with an external `<label htmlFor>` — a credit row's
   *  role, which sits outside this component's own markup. */
  id?: string
  /** How much room the content actually needs. Omitted for the genuinely
   *  unbounded — a URL, a search — which fill what they're given. */
  width?: string
}) {
  return (
    <input
      id={id}
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
  ariaLabel,
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
  /** For a select whose name is a heading elsewhere — a catalog section's
   *  head — rather than a label of its own. */
  ariaLabel?: string
}) {
  const select = (
    <select
      aria-label={ariaLabel}
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
  //
  // On touch the box grows from that same pinned right edge, so it grows
  // leftward and away from the chevron rather than over it. A 16px target
  // sitting inside a control that opens a picker is the one size that is worse
  // than no target at all: the miss doesn't do nothing, it opens the list.
  // `select.field--clearable`'s padding grows to match — the two are in
  // `index.css` and here, and have to move together.
  return (
    <div className="relative w-full" style={{ maxWidth: width }}>
      {select}
      {value && (
        <button
          type="button"
          aria-label={`Clear ${clearLabel}`}
          onClick={() => onChange('')}
          className="text-dimmer hover:text-ink absolute top-1/2 right-[27px] grid h-4 w-4 -translate-y-1/2 place-items-center text-[13px] leading-none transition-colors pointer-coarse:h-9 pointer-coarse:w-9"
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
 * **It sizes itself.** `w-fit` plus `whitespace-nowrap` means no caller has to
 * guess a pixel width for the labels it holds — a label always gets the room
 * it needs rather than wrapping or losing its tail.
 *
 * **Which is why a narrow screen wraps the group rather than shrinking it.**
 * `w-fit` cannot shrink, so on a screen too narrow for the group's natural
 * width, `max-w-full` with `flex-wrap` takes a second line instead of
 * overflowing the container — the labels stay whole, which is the property
 * this control exists to keep.
 */
export function Segmented<T extends string>({
  value,
  onChange,
  options,
  ariaLabel,
  disabled = false,
}: {
  value: T
  onChange: (value: T) => void
  options: { value: T; label: string }[]
  ariaLabel: string
  /** Greyed as a whole while the control can't do anything — the "All" tab
   *  segmented control outside Tabbed Grids. Keeps its value: the selected
   *  segment stays focusable (aria-disabled) with a not-allowed cursor and
   *  shows the kept value underlined rather than filled, so it reads as "on,
   *  but inert" rather than "cleared". The reason lives in a grey note beside
   *  it, which the caller supplies — this control only draws the grey state. */
  disabled?: boolean
}) {
  return (
    <div
      role="group"
      aria-label={ariaLabel}
      aria-disabled={disabled || undefined}
      className="border-line-hi flex w-fit max-w-full shrink-0 flex-wrap overflow-hidden rounded-[2px] border"
    >
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          onClick={() => !disabled && onChange(option.value)}
          aria-pressed={value === option.value}
          aria-disabled={disabled || undefined}
          className={`border-line-hi flex-1 border-r px-3 py-1.5 text-[10.5px] tracking-[0.08em] whitespace-nowrap uppercase transition-colors last:border-r-0 pointer-coarse:py-2.5 ${
            disabled
              ? `text-dimmer cursor-not-allowed ${value === option.value ? 'underline decoration-1 underline-offset-[0.3em]' : ''}`
              : `hover:text-ink ${value === option.value ? 'bg-raised-hi text-ink' : 'text-dim'}`
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
 * drag on either thumb produces both values at once. Reporting them through
 * two callbacks risks a consumer that writes the pair as one object: the
 * second call would overwrite the first with a stale closure's value, and a
 * thumb wired that way could never move. A control that moves two values
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
    // `--thumb` rather than a literal, because the thumb's size is read in two
    // places that have to agree: the thumbs themselves, and `ThumbValue`'s
    // `calc`, which centres a caption on a thumb whose centre travels between
    // half a thumb-width in from each end. Grown on touch — 13px is a target
    // you cannot put a finger on, and for the age-rating scale the slider is
    // the only control there is. The root grows with it so the taller thumb
    // has room and the whole band is grabbable, not just the 3px track.
    <div className="flex max-w-[var(--w-track)] flex-col gap-1.5 [--thumb:13px] pointer-coarse:[--thumb:24px]">
      <Slider.Root
        className="relative flex h-4 w-full touch-none items-center select-none pointer-coarse:h-11"
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
          className="border-dim bg-raised-hi hover:border-ink focus-visible:ring-ink block h-[var(--thumb)] w-[var(--thumb)] rounded-full border shadow-sm outline-none focus-visible:ring-2"
        />
        <Slider.Thumb
          aria-label="Maximum"
          className="border-dim bg-raised-hi hover:border-ink focus-visible:ring-ink block h-[var(--thumb)] w-[var(--thumb)] rounded-full border shadow-sm outline-none focus-visible:ring-2"
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
 *
 * Half a thumb-width is `--thumb`, inherited from the slider's wrapper, not a
 * literal: the thumb is bigger on touch, and a caption still measuring the
 * mouse-sized one would sit off its own thumb at exactly the ends this `calc`
 * exists to get right.
 */
function ThumbValue({ fraction, label }: { fraction: number; label: string }) {
  return (
    <span
      style={{ left: `calc(${fraction * 100}% + ${0.5 - fraction} * var(--thumb))` }}
      className="type-data text-dimmer absolute top-0 -translate-x-1/2 text-[10.5px] leading-none whitespace-nowrap"
    >
      {label}
    </span>
  )
}

/**
 * The app's one switch, scoped to sharing (DESIGN.md, "Sharing switch"). A
 * catalog's or a collection's Sharing row is the only setting that uses one —
 * every other yes/no in the app is a segmented control, because a switch
 * flips state on its own while a segment shows every value at once, and
 * "shared or not" is the one setting whose current value is worth reading at
 * a glance without the words beside it (which this still carries — the state
 * is never colour or position alone).
 */
export function Switch({
  checked,
  onChange,
  label,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  /** The state in words, beside the track — "Shared, so anyone can import
   *  it" / "Not shared, only you can use it". Never omitted: the switch has
   *  no label of its own. */
  label: string
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className="switch"
    >
      <span className="switch-track">
        <span className="switch-knob" />
      </span>
      <span>{label}</span>
    </button>
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
