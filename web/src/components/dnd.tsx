import {
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core'
import { sortableKeyboardCoordinates, useSortable } from '@dnd-kit/sortable'
import { ChevronDown, ChevronUp } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { GripIcon, Icon } from './Icon'

/**
 * The three sensors every drag-to-reorder list in the app shares — the Home
 * pane's two lists (`SortableList`) and the collection builder's folder tree
 * (`FolderTreeDnd`).
 *
 *  - Mouse, with a small activation distance so a click on a control inside
 *    a row still registers as a click rather than starting a drag. Mouse, not
 *    Pointer: `pointerdown` fires before `touchstart`, so a pointer sensor
 *    claims every touch first, and the browser cancels its pointer stream as
 *    soon as the finger starts a pan — the drag dies on Android.
 *  - Touch, on a hold rather than a distance: 4px of movement is the start of
 *    a scroll on a finger, not the start of a drag, and a grip inside a
 *    scrolling page has to let a swipe through — which is why the grips drop
 *    `touch-none` for a `touch-manipulation` that lets a quick swipe through.
 *  - Keyboard, the only way any of these lists is operable for anyone who
 *    can't drag.
 */
export function useDragSensors() {
  return useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 4 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 200, tolerance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )
}

/** Drag listeners go on the grip alone, never the whole row — a row can be
 *  full of buttons and inputs, and a row-wide drag surface would swallow
 *  every click in it. Typed off `useSortable`'s own return so dnd-kit owns
 *  the shape. */
export function Grip({
  label,
  sortable,
}: {
  label: string
  sortable: Pick<ReturnType<typeof useSortable>, 'attributes' | 'listeners'>
}) {
  return (
    <button
      type="button"
      aria-label={label}
      className="tap text-dimmer hover:text-dim grid cursor-grab touch-none place-items-center leading-none transition-colors active:cursor-grabbing pointer-coarse:touch-manipulation"
      {...sortable.attributes}
      {...sortable.listeners}
    >
      <GripIcon />
    </button>
  )
}

/** A 32px grey icon button for a row's ↑/↓ (or any other single-icon action),
 *  greying to the dimmest text colour with no hover response when `disabled` — the
 *  first row's ↑ and the last row's ↓ in any reorderable list. Shared by the
 *  Home pane's running-order rows (↑/↓) and the collection editor's selected
 *  folder (←/→). */
export function RowIconButton({
  icon,
  label,
  disabled,
  onClick,
}: {
  icon: LucideIcon
  label: string
  disabled: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-label={label}
      title={label}
      className={`tap grid h-8 w-8 place-items-center rounded-full transition-colors ${
        disabled
          ? 'text-dimmer cursor-not-allowed'
          : 'text-dim hover:bg-line hover:text-ink active:bg-line-hi'
      }`}
    >
      <Icon icon={icon} size={14} />
    </button>
  )
}

export function MoveUpButton(props: Omit<Parameters<typeof RowIconButton>[0], 'icon'>) {
  return <RowIconButton icon={ChevronUp} {...props} />
}

export function MoveDownButton(props: Omit<Parameters<typeof RowIconButton>[0], 'icon'>) {
  return <RowIconButton icon={ChevronDown} {...props} />
}
