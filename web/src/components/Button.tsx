import type { ComponentProps } from 'react'

/** Each kind's classes in `index.css`'s `@layer components` (DESIGN.md, "Buttons" and "Signs"). */
const KIND = {
  primary: 'btn-primary',
  push: 'btn-push',
  secondary: 'btn-secondary',
  ghost: 'btn-ghost',
  danger: 'btn-danger',
  'danger-text': 'btn-danger-text',
  'accent-outline': 'btn-secondary btn-accent-outline',
  sign: 'sign-btn',
  'sign-outline': 'sign-btn-outline',
} as const

export type ButtonKind = keyof typeof KIND

/**
 * The one text button: `kind` picks its look from the classes `index.css`
 * already defines, and `size="sm"` (30px, 44 on touch) is for actions inside
 * panes, rows and empty states. The sign kinds have one size of their own.
 *
 * A caller's utility class overrides the kind (utilities outrank the
 * components layer), so a one-off width or margin needs no variant. Every
 * other button prop passes through, `ref` and `aria-disabled` included, so a
 * greyed Save stays pressable and a Radix trigger can render it through
 * `asChild`.
 */
export function Button({
  kind,
  size = 'md',
  className = '',
  ...rest
}: ComponentProps<'button'> & {
  kind: ButtonKind
  size?: 'md' | 'sm'
}) {
  const small = size === 'sm' && !kind.startsWith('sign') ? ' btn-sm' : ''
  return (
    <button
      type="button"
      data-slot="button"
      {...rest}
      className={`${KIND[kind]}${small} ${className}`.trimEnd()}
    />
  )
}
