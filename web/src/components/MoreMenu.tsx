import type { ComponentProps, ReactNode } from 'react'
import { MoreHorizontal } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import { IconButton } from '@/components/IconButton'

/**
 * A row's "⋯" menu: the square trigger, named "More for {label}", and the
 * panel it opens, end-aligned under it. Its entries are `MoreMenuItem`s,
 * grouped by `MoreMenuSeparator`.
 */
export function MoreMenu({ label, children }: { label: string; children: ReactNode }) {
  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger asChild>
        <IconButton label={`More for ${label}`} icon={MoreHorizontal} variant="plain" size="sm" />
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="end"
          sideOffset={4}
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
