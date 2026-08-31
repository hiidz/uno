import { useEffect, useId, useRef, useState } from 'react'
import { useCopy } from '@/features/push/useCopy'

/**
 * The profile chip below `lg`, and the two profile-scoped actions behind it.
 *
 * Above `lg` the chip is a single button that switches profile and this
 * component isn't rendered — see `Builder`. Stacked, the header is one row that
 * must not wrap, and the chip, the addon URL button and Push don't fit beside
 * the pending badge on a 320px screen. Both of the movable ones are about *this
 * profile* — the addon URL is this profile's manifest — so the chip that
 * already names the profile carries them, rather than a generic overflow button
 * added beside it.
 *
 * Two costs, both accepted: switching profile is two taps here rather than one,
 * and the addon URL is a tap further away than it was before a first push,
 * which is the only point at which `PushBanner` isn't also offering it.
 *
 * The chip truncates rather than wrapping. Profile names come from Nuvio and
 * have no length limit, so this is the flex item that absorbs whatever the row
 * doesn't have room for; the menu shows the name in full.
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
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const triggerId = useId()

  // Always called, because hooks are: the copy item is only rendered when
  // there's a URL, so the empty string is never reachable from the UI.
  const { copied, copy } = useCopy(manifestURL ?? '')

  useEffect(() => {
    if (!open) return

    // A tap anywhere else dismisses. `pointerdown` rather than `click` so the
    // menu is gone before the thing underneath reacts.
    function onPointerDown(event: PointerEvent) {
      if (rootRef.current?.contains(event.target as Node)) return
      setOpen(false)
    }

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        setOpen(false)
        triggerRef.current?.focus()
        return
      }
      // Arrow keys are what `role="menu"` promises, so they're implemented
      // rather than claimed. Two items, so this only ever wraps between them.
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        event.preventDefault()
        stepFocus(menuRef.current, event.key === 'ArrowDown' ? 1 : -1)
      }
    }

    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  // Opening moves focus in, so Escape has somewhere to send it back from and a
  // keyboard user isn't left behind on the trigger.
  useEffect(() => {
    if (!open) return
    menuRef.current?.querySelector<HTMLElement>('[role="menuitem"]')?.focus()
  }, [open])

  function switchProfile() {
    setOpen(false)
    // Straight through to the header's own guard — unchanged, including the
    // unpushed-changes confirmation it may raise instead of leaving.
    onSwitchProfile()
  }

  return (
    <div ref={rootRef} className={`relative min-w-0 ${className}`}>
      <button
        ref={triggerRef}
        id={triggerId}
        type="button"
        onClick={() => setOpen((previous) => !previous)}
        aria-haspopup="menu"
        aria-expanded={open}
        // Names the profile *and* the control. The visible text is the profile
        // alone, and truncated at that, so on its own it would announce a
        // number and a name with no hint that anything opens.
        aria-label={`Profile menu — profile ${profileIndex}, ${profileName}`}
        // `max-w-full` is load-bearing: a button sizes to its own content even
        // as a flex container, so without it this keeps its full width and
        // spills out of the wrapper the row has already squeezed — over the
        // pending badge — instead of truncating inside it.
        className="type-data border-line-hi text-dim hover:text-ink hover:border-dim flex min-w-0 max-w-full items-center gap-1 rounded-[2px] border px-2 py-[3px] text-[11px] transition-colors"
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
      </button>

      {open && (
        // `z-40` deliberately: over the header's own `z-30`, under `Modal` and
        // `ConfirmDialog` at `z-50` — including the discard prompt that
        // switching profile can raise.
        <div
          ref={menuRef}
          role="menu"
          aria-labelledby={triggerId}
          className="bg-raised-hi border-line-hi absolute top-full left-0 z-40 mt-1.5 flex w-[14rem] max-w-[calc(100vw-2rem)] flex-col gap-0.5 rounded-[2px] border p-1.5 shadow-[0_12px_28px_rgba(0,0,0,0.55)]"
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

          <MenuItem
            label="Switch profile"
            detail="Goes back to the picker. Warns first if anything is unpushed."
            onClick={switchProfile}
          />

          {manifestURL && (
            <MenuItem
              label={copied ? 'Copied' : 'Copy addon url'}
              detail="Install this in Nuvio to get your home screen."
              // Stays open on copy. The confirmation is this item's own label,
              // and closing would take it off screen before it could be read —
              // there is no toast here to report it anywhere else.
              onClick={copy}
            />
          )}
        </div>
      )}
    </div>
  )
}

function MenuItem({
  label,
  detail,
  onClick,
}: {
  label: string
  detail: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      role="menuitem"
      tabIndex={-1}
      onClick={onClick}
      className="hover:bg-line focus-visible:bg-line flex min-h-[44px] flex-col justify-center gap-0.5 rounded-[2px] px-2 py-2 text-left transition-colors"
    >
      <span className="type-data text-ink text-[11.5px]">{label}</span>
      {/* What the desktop button says in its `title`. Kept as text because
          `title` never surfaces on touch, which is the only pointer that
          reaches this menu. */}
      <span className="type-data text-dimmer text-[10px] leading-snug">{detail}</span>
    </button>
  )
}

/** Roving focus between the menu's items. Reads the DOM rather than tracking an
 *  index, so an item that isn't rendered — the copy, without a URL — simply
 *  isn't in the rotation. */
function stepFocus(menu: HTMLElement | null, delta: number) {
  const items = [...(menu?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])]
  if (items.length === 0) return
  const current = items.indexOf(document.activeElement as HTMLElement)
  items[(current + delta + items.length) % items.length]?.focus()
}
