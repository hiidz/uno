import { DropdownMenu } from 'radix-ui'
import { useCopy } from '@/features/push/useCopy'
import { useStackedLayout } from './stacked'

/**
 * The profile chip, in two shapes depending on how much room the header has.
 *
 * **Above `lg`** the header has room for the addon URL button and Push beside
 * the chip, so the chip itself is a plain button that switches profile —
 * nothing behind it needs a menu.
 *
 * **Below `lg`** the chip becomes the trigger for a menu carrying the two
 * profile-scoped actions. Stacked, the header is one row that must not wrap,
 * and the chip, the addon URL button and Push don't fit beside the pending
 * badge on a 320px screen. Both of the movable ones are about *this
 * profile* — the addon URL is this profile's manifest — so the chip that
 * already names the profile carries them, rather than a generic overflow
 * button added beside it.
 *
 * Two costs, both accepted: switching profile is two taps below `lg` rather
 * than one, and the addon URL is a tap further away than it was before a
 * first push, which is the only point at which `PushBanner` isn't also
 * offering it.
 *
 * Which shape renders is read from `useStackedLayout`, the same query
 * `Workspace` and `EditorShell` use for the same breakpoint — not `lg:hidden`
 * / `hidden lg:block` on two separate elements, which would mean two
 * components doing the one job. Only one shape is ever in the DOM: stronger
 * than `display: none` for keeping the other out of the accessibility tree,
 * since it's absent rather than merely hidden.
 *
 * The chip truncates rather than wrapping. Profile names come from Nuvio and
 * have no length limit, so this is the flex item that absorbs whatever the row
 * doesn't have room for; the menu (below `lg`) shows the name in full.
 *
 * **Menu, dismiss, and keyboard navigation are Radix's `DropdownMenu`**, not
 * hand-rolled — role, focus management, Escape, and outside-dismiss all come
 * from the primitive. `modal={false}` keeps it non-blocking: no scroll lock
 * and no hiding the rest of the page from assistive tech, matching what this
 * chip has always done. `loop` keeps Up/Down wrapping between the two items.
 * No `DropdownMenu.Portal`: this stays in normal DOM flow rather than
 * escaping to `document.body`, which is what keeps it under the header's own
 * `z-40` without a portal's stacking context to reconcile against `Modal` and
 * `ConfirmDialog` at `z-50`.
 */
export function ProfileMenu({
  profileIndex,
  profileName,
  manifestURL,
  onSwitchProfile,
  className = '',
}: {
  profileIndex: number
  profileName: string
  /** Absent when navigation state didn't carry it, in which case there is
   *  nothing to copy and the menu holds only the switch. */
  manifestURL?: string
  onSwitchProfile: () => void
  className?: string
}) {
  const stacked = useStackedLayout()

  // Always called, because hooks are: the copy item is only rendered when
  // there's a URL, so the empty string is never reachable from the UI.
  const { copied, copy } = useCopy(manifestURL ?? '')

  // Above `lg` there's room for the addon URL button and Push beside the
  // chip, so the chip needs no menu behind it — a direct switch is the
  // one-tap control it's always been.
  if (!stacked) {
    return (
      <button
        type="button"
        onClick={onSwitchProfile}
        title="Switch profile"
        className={`type-data border-line-hi text-dim hover:text-ink hover:border-dim shrink-0 rounded-[2px] border px-2 py-[3px] text-[11px] whitespace-nowrap transition-colors ${className}`}
      >
        profile {profileIndex} · {profileName}
      </button>
    )
  }

  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger
        // Names the profile *and* the control. The visible text is the profile
        // alone, and truncated at that, so on its own it would announce a
        // number and a name with no hint that anything opens.
        aria-label={`Profile menu — profile ${profileIndex}, ${profileName}`}
        // `max-w-full` is load-bearing: a button sizes to its own content even
        // as a flex container, so without it this keeps its full width and
        // spills out of the wrapper the row has already squeezed — over the
        // pending badge — instead of truncating inside it.
        className={`type-data border-line-hi text-dim hover:text-ink hover:border-dim flex min-w-0 max-w-full items-center gap-1 rounded-[2px] border px-2 py-[3px] text-[11px] transition-colors ${className}`}
      >
        {/* The word "profile" is dropped here and kept in the menu below: it is
            52.9px of a row that has none to spare, and the chip's position and
            the menu it opens both already say what it is. */}
        <span className="truncate">
          {profileIndex} · {profileName}
        </span>
        <span aria-hidden="true" className="shrink-0 text-[9px] leading-none">
          ▾
        </span>
      </DropdownMenu.Trigger>

      {/* No `Portal`: see the module comment. `avoidCollisions` is on — an
          explicit choice, not an inherited default — since nothing here stops
          the menu running off a short or narrow viewport otherwise. */}
      <DropdownMenu.Content
        loop
        side="bottom"
        align="start"
        sideOffset={6}
        avoidCollisions
        className="bg-raised-hi border-line-hi z-40 flex w-[14rem] max-w-[calc(100vw-2rem)] flex-col gap-0.5 rounded-[2px] border p-1.5 shadow-[0_12px_28px_rgba(0,0,0,0.55)]"
      >
        {/* The name in full, which the chip itself may have truncated.
            `aria-hidden` because the trigger's label already carries it and a
            menu should read as its items. */}
        <p
          aria-hidden="true"
          className="type-data text-dimmer border-line m-0 truncate border-b px-2 py-2 text-[10px]"
        >
          profile {profileIndex} · {profileName}
        </p>

        {/* Straight through to the header's own guard — unchanged, including
            the unpushed-changes confirmation it may raise instead of leaving.
            No `preventDefault`, so Radix's default select-closes-the-menu
            behaviour applies here. */}
        <MenuItem
          label="Switch profile"
          detail="Goes back to the picker. Warns first if anything is unpushed."
          onSelect={onSwitchProfile}
        />

        {manifestURL && (
          <MenuItem
            label={copied ? 'Copied' : 'Copy addon url'}
            detail="Install this in Nuvio to get your home screen."
            // Stays open on copy. The confirmation is this item's own label,
            // and closing would take it off screen before it could be read —
            // there is no toast here to report it anywhere else. Radix closes
            // on select by default, so this is the one item that opts out.
            onSelect={(event) => {
              event.preventDefault()
              copy()
            }}
          />
        )}
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  )
}

function MenuItem({
  label,
  detail,
  onSelect,
}: {
  label: string
  detail: string
  onSelect: (event: Event) => void
}) {
  return (
    <DropdownMenu.Item
      onSelect={onSelect}
      className="hover:bg-line focus-visible:bg-line flex min-h-[44px] flex-col justify-center gap-0.5 rounded-[2px] px-2 py-2 text-left transition-colors"
    >
      <span className="type-data text-ink text-[11.5px]">{label}</span>
      {/* What the desktop button says in its `title`. Kept as text because
          `title` never surfaces on touch, which is the only pointer that
          reaches this menu. */}
      <span className="type-data text-dimmer text-[10px] leading-snug">{detail}</span>
    </DropdownMenu.Item>
  )
}
