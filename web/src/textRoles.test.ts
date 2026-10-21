/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

/** The text roles as approved (DESIGN.md, "Typography"): size, line height, weight. */
const ROLES: Record<string, [size: string, lineHeight: string, weight?: string]> = {
  sticker: ['10.5px', '1', '700'],
  caption: ['12px', '1.4'],
  meta: ['13px', '1.45'],
  label: ['13.5px', '1.3', '650'],
  'body-sm': ['14px', '1.45'],
  body: ['15px', '1.5'],
  title: ['16px', '1.3', '700'],
  display: ['21px', '28px', '750'],
  sign: ['12.5px', '1', '800'],
  'sign-headline': ['16px', '1.15', '800'],
  'sign-title': ['18px', '1.25', '800'],
  'sign-hero': ['34px', '1.05', '800'],
  'sign-hero-wide': ['38px', '1.05', '800'],
}

const css = readFileSync(new URL('./index.css', import.meta.url), 'utf8').replaceAll('\r\n', '\n')

const theme = css.match(/@theme static \{([\s\S]*?)\n\}/)?.[1] ?? ''

function declared(name: string): string | undefined {
  return theme.match(new RegExp(`^\\s*--text-${name}:\\s*([^;]+);`, 'm'))?.[1]
}

function inMedia(query: string): string {
  const at = css.indexOf(`@media (${query}) {\n    :root {`)
  return at < 0 ? '' : css.slice(at, css.indexOf('}', at))
}

describe('text roles', () => {
  it.each(Object.entries(ROLES))('%s sets its size, line height and weight', (role, [size, lineHeight, weight]) => {
    expect(declared(role)).toBe(size)
    expect(declared(`${role}--line-height`)).toBe(lineHeight)
    expect(declared(`${role}--font-weight`)).toBe(weight)
  })

  it('declares no role beyond the approved ones and the touch field size', () => {
    const roles = [...theme.matchAll(/^\s*--text-([a-z-]+):/gm)].map((m) => m[1]).filter((name) => !name.includes('--'))
    expect(roles.sort()).toEqual([...Object.keys(ROLES), 'field'].sort())
    expect(declared('field')).toBe('16px')
  })

  it('grows the pane signs from lg and shrinks the sign-in headline below sm', () => {
    expect(inMedia('width >= 64rem')).toContain('--text-sign-headline: 25px;')
    expect(inMedia('width >= 64rem')).toContain('--text-sign-title: 25px;')
    expect(inMedia('width < 40rem')).toContain('--text-sign-hero: 25px;')
    expect(inMedia('width < 40rem')).toContain('--text-sign-hero-wide: 27px;')
  })
})
