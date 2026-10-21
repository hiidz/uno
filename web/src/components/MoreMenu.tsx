import { useRef, type ComponentProps, type ReactNode } from 'react'
import { MoreHorizontal } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import { IconButton } from '@/components/IconButton'
import { focusIsLost } from './returnFocus'

/**
 * A row's "⋯" menu: the square trigger, named "More for {label}", and the
 * panel it opens, end-aligned under it. Its entries are `MoreMenuItem`s,
 * grouped by `MoreMenuSeparator`.
 *
 * On close Radix puts focus back on the trigger. An entry that takes its row
 * away (Remove from home, Remove from folder) or moves it to another list
 * (Move to Discover, Pin) takes the trigger with it, so focus goes to
 * `nextFocus`'s pick instead.
 */
export function MoreMenu({ label, children }: { label: string; children: ReactNode }) {
  const name = `More for ${label}`
  const trigger = useRef<HTMLElement | null>(null)
  const near = useRef<HTMLElement[]>([])
  return (
    <DropdownMenu.Root
      modal={false}
      onOpenChange={(open) => {
        if (open && trigger.current) near.current = triggersNear(trigger.current)
      }}
    >
      <DropdownMenu.Trigger asChild>
        <IconButton
          ref={(element) => {
            if (element) trigger.current = element
          }}
          label={name}
          icon={MoreHorizontal}
          variant="plain"
          size="sm"
          data-more-menu=""
        />
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="end"
          sideOffset={4}
          onCloseAutoFocus={(event) => {
            const own = trigger.current
            if (own?.isConnected || !focusIsLost()) return
            event.preventDefault()
            nextFocus(name, own, near.current)?.focus({ preventScroll: true })
          }}
          className="bg-raised-hi border-line-hi z-40 flex w-60 flex-col gap-0.5 rounded-xl border p-1.5"
        >
          {children}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

/** One entry. `danger` letters it in danger red, for an action that can't be
 *  taken back; `reason` says why, dim at the entry's end. */
export function MoreMenuItem({
  danger = false,
  reason,
  children,
  ...props
}: Omit<ComponentProps<typeof DropdownMenu.Item>, 'className'> & {
  danger?: boolean
  reason?: string
}) {
  return (
    <DropdownMenu.Item
      {...props}
      className={`hover:bg-line focus-visible:bg-line data-[disabled]:text-dimmer data-[disabled]:hover:bg-transparent flex items-center justify-between gap-2 rounded-lg px-2.5 py-2 text-left text-body-sm font-medium transition-colors pointer-coarse:min-h-11 ${
        danger ? 'text-danger' : 'text-ink'
      }`}
    >
      {children}
      {reason && <span className="text-dimmer text-meta">{reason}</span>}
    </DropdownMenu.Item>
  )
}

export function MoreMenuSeparator() {
  return <DropdownMenu.Separator className="bg-line my-1 h-px" />
}

/** The "⋯" triggers in the same list as `trigger`, in page order. */
function triggersNear(trigger: HTMLElement): HTMLElement[] {
  const list = trigger.closest('ul, ol') ?? document.body
  return [...list.querySelectorAll<HTMLElement>('[data-more-menu]')]
}

/**
 * Where focus goes once a menu's trigger has left the page: the same row's
 * new "⋯" where the row moved to another list, else the nearest "⋯" left in
 * its list, the next one before the one above it.
 */
function nextFocus(name: string, gone: HTMLElement | null, near: HTMLElement[]): HTMLElement | undefined {
  const moved = [...document.querySelectorAll<HTMLElement>('[data-more-menu]')].find(
    (element) => element.getAttribute('aria-label') === name,
  )
  if (moved) return moved
  const at = gone ? near.indexOf(gone) : -1
  if (at < 0) return undefined
  const after = near.slice(at + 1).find((element) => element.isConnected)
  return after ?? near.slice(0, at).reverse().find((element) => element.isConnected)
}
