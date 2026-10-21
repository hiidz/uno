/// <reference types="node" />
import { readdirSync, readFileSync } from 'node:fs'
import { join, relative } from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * The drift check (docs/frontend.md, "UI rules"): a screen file draws its text
 * through the text roles, its buttons through `Button` and `IconButton`, and
 * its colours through the colour tokens. It matches literal source only, so it
 * nudges rather than guarantees.
 *
 * `components/` is where the raw forms live, so it is not checked. A
 * deliberate one-off carries `drift-ok: <reason>` on its own line or the line
 * above, as a `//` or `{/* *\/}` comment.
 */
const RULES = [
  { name: 'a raw text size, where a text role belongs', pattern: /text-\[[\d.]+px\]/ },
  { name: 'a raw <button>, where Button or IconButton belongs', pattern: /<button\b/ },
  { name: 'a raw colour, where a colour token belongs', pattern: /\[#[\da-fA-F]{3,8}\]|\brgba?\(/ },
]

const OPT_OUT = /drift-ok:\s*\S/

/** Screens not yet moved onto the components and roles. Adopt takes each off as it moves it. */
const NOT_YET_MIGRATED = new Set([
  'features/account/KeyProblemBanner.tsx',
  'features/account/TMDBKey.tsx',
  'features/bundle/ExportDialog.tsx',
  'features/bundle/ImportDialog.tsx',
  'features/bundle/ImportReview.tsx',
  'features/bundle/JsonField.tsx',
  'features/catalogs/CatalogEditor.tsx',
  'features/catalogs/DateWindow.tsx',
  'features/catalogs/RecipePreview.tsx',
  'features/catalogs/TMDBEntityPicker.tsx',
  'features/catalogs/WatchProviderPicker.tsx',
  'features/catalogs/fields.tsx',
  'features/community/CommunityRow.tsx',
  'features/community/CommunitySign.tsx',
  'features/community/CommunityView.tsx',
  'features/community/PublicationPage.tsx',
  'features/community/RowPreview.tsx',
  'features/community/SearchNote.tsx',
  'features/community/ShowMore.tsx',
  'features/preview/tiles.tsx',
  'features/sharing/CatalogBlock.tsx',
  'features/sharing/CatalogList.tsx',
  'features/sharing/ChangeList.tsx',
  'features/sharing/Changes.tsx',
  'features/sharing/FromCommunityView.tsx',
  'features/sharing/MarkViews.tsx',
  'features/sharing/MarkedTiles.tsx',
  'features/sharing/PublicationBodies.tsx',
  'features/sharing/PublishDialog.tsx',
  'routes/ErrorPage.tsx',
  'routes/Login.tsx',
  'routes/ProfilePicker.tsx',
  'routes/ProfilesRetry.tsx',
])

const SRC = join(import.meta.dirname)

function screenFiles(): string[] {
  return readdirSync(SRC, { recursive: true, encoding: 'utf8' })
    .map((path) => path.replaceAll('\\', '/'))
    .filter((path) => path.endsWith('.tsx') && !path.endsWith('.test.tsx') && !path.startsWith('components/'))
    .sort()
}

function drift(path: string): string[] {
  return driftIn(path, readFileSync(join(SRC, path), 'utf8'))
}

/** Each line breaking a rule without an opt-out, as `file:line: rule`. */
function driftIn(path: string, source: string): string[] {
  const lines = source.split(/\r?\n/)
  return lines.flatMap((line, i) =>
    RULES.filter(({ pattern }) => pattern.test(line))
      .filter(() => !OPT_OUT.test(line) && !OPT_OUT.test(lines[i - 1] ?? ''))
      .map(({ name }) => `${path}:${i + 1}: ${name}`),
  )
}

describe('drift check', () => {
  const files = screenFiles()

  it('flags a raw size, a raw button and a raw colour, each on its own line', () => {
    const source = [
      '<p className="text-[13px]">',
      '<button type="button">',
      '<span className="bg-[#FFC93C]">',
      '<span style={{ color: "rgb(0 0 0)" }}>',
      '<p className="text-meta">',
    ].join('\n')
    expect(driftIn('x.tsx', source).map((hit) => hit.split(':')[1])).toEqual(['1', '2', '3', '4'])
  })

  it('lets a line through with a reasoned opt-out on it or above it, never a bare one', () => {
    const source = [
      '{/* drift-ok: emoji sized to its tile */}',
      '<span className="text-[26px]">',
      '<span className="text-[24px]"> {/* drift-ok: emoji sized to its tile */}',
      '// drift-ok:',
      '<span className="text-[18px]">',
    ].join('\n')
    expect(driftIn('x.tsx', source)).toEqual(['x.tsx:5: a raw text size, where a text role belongs'])
  })

  it('finds the screen files', () => {
    expect(files.length).toBeGreaterThan(50)
    expect(relative(SRC, join(SRC, files[0]))).not.toMatch(/^components/)
  })

  it('holds every migrated screen to the components, text roles and colour tokens', () => {
    const found = files.filter((path) => !NOT_YET_MIGRATED.has(path)).flatMap(drift)
    expect(found).toEqual([])
  })

  it('lists only screens that still drift, so a migrated one comes off the list', () => {
    const clean = [...NOT_YET_MIGRATED].filter((path) => !files.includes(path) || drift(path).length === 0)
    expect(clean).toEqual([])
  })
})
