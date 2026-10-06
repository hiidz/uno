// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/api/http'
import type { ImportCatalog, ImportCheck } from '@/api'
import { ImportDialog } from './ImportDialog'
import { MAX_BUNDLE_BYTES } from './text'

const api = vi.hoisted(() => ({ checkImport: vi.fn(), importBundle: vi.fn() }))
vi.mock('@/api', async () => ({
  queryKeys: (await import('@/api/keys')).queryKeys,
  tmdbKind: (await import('@/api/types')).tmdbKind,
  ...api,
}))
vi.mock('@/api/client', () => ({ apiFetch: vi.fn() }))

const BUNDLE = { format: 'uno-bundle', catalogs: [{ name: 'A' }, { name: 'B' }] }
const POPULAR = '{"sort_by":"popularity.desc"}'
/** A bundle catalog the check lists, matching the library's catalogs `existing`. */
function catalog(
  key: string,
  name: string,
  existing: string[] = [],
  type: ImportCatalog['type'] = 'movie',
  params = POPULAR,
): ImportCatalog {
  return { key, name, type, params, existing: existing.map((id) => ({ id, name: `My ${name}` })) }
}

const CHECK: ImportCheck = {
  catalogs: [catalog('c1', 'A'), catalog('c2', 'B', [], 'series', '{"sort_by":"vote_average.desc"}')],
  collections: [],
}
const GENRES = { movie: new Map<number, string>(), tv: new Map<number, string>() }
const MATCHED: ImportCheck = { ...CHECK, catalogs: [catalog('c1', 'A', ['mine-1']), CHECK.catalogs[1]] }
/** A bundle of top-level c1 and the collection Halloween, holding s1 and s2:
 *  Halloween matches by title, c1 and s1 by recipe. */
const HALLOWEEN: ImportCheck = {
  catalogs: [MATCHED.catalogs[0]],
  collections: [
    {
      title: 'Halloween',
      folders: ['Scary', 'Cosy'],
      matched: true,
      catalogs: [catalog('s1', 'S', ['mine-2']), catalog('s2', 'T')],
    },
  ],
}
const RESULT = { catalogs: [], collections: [] }

function renderDialog() {
  const onImported = vi.fn()
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ImportDialog open profileIndex={3} genres={GENRES} onClose={vi.fn()} onImported={onImported} />
    </QueryClientProvider>,
  )
  return { onImported }
}

const field = () => screen.getByRole('textbox', { name: 'Import JSON' })
const press = (name: string) => fireEvent.click(screen.getByRole('button', { name }))

function paste(text: string) {
  fireEvent.change(field(), { target: { value: text } })
}

beforeEach(() => {
  api.checkImport.mockReset()
  api.importBundle.mockReset()
  api.checkImport.mockResolvedValue(CHECK)
  api.importBundle.mockResolvedValue(RESULT)
})

describe('ImportDialog', () => {
  it('opens on an empty field with Validate JSON and Import off', () => {
    renderDialog()
    expect(field()).toHaveValue('')
    expect(screen.getByRole('button', { name: 'Validate JSON' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Import' })).toBeDisabled()
    expect(screen.queryByText(/file/i)).not.toBeInTheDocument()
  })

  it('keeps Validate JSON and Import off while the box is blank', () => {
    renderDialog()
    paste('   ')
    expect(screen.getByRole('button', { name: 'Validate JSON' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Import' })).toBeDisabled()
  })

  it('numbers the lines of the pasted text', () => {
    renderDialog()
    paste('{\n"a": 1\n}')
    expect(field().previousElementSibling?.textContent).toMatch(/^1\n2\n3\n4/)
  })

  it('validates in the browser alone: valid JSON is said so, and nothing is sent', () => {
    renderDialog()
    paste(JSON.stringify(BUNDLE))
    press('Validate JSON')

    expect(screen.getByText('Valid JSON.')).toBeInTheDocument()
    expect(field()).toHaveValue(JSON.stringify(BUNDLE))
    expect(api.checkImport).not.toHaveBeenCalled()
    expect(api.importBundle).not.toHaveBeenCalled()

    fireEvent.change(field(), { target: { value: '{}' } })
    expect(screen.queryByText('Valid JSON.')).not.toBeInTheDocument()
  })

  it('refuses invalid JSON in place, keeps the text, sends nothing, and clears on the next edit', () => {
    renderDialog()
    paste('{nope')
    press('Validate JSON')

    expect(screen.getByRole('alert')).toHaveTextContent(/^This isn't valid JSON\.\S/)
    expect(field()).toHaveValue('{nope')
    expect(api.checkImport).not.toHaveBeenCalled()

    fireEvent.change(field(), { target: { value: '{' } })
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('refuses invalid JSON on Import without sending it', () => {
    renderDialog()
    paste('{nope')
    press('Import')

    expect(screen.getByRole('alert')).toHaveTextContent(/^This isn't valid JSON\.\S/)
    expect(api.checkImport).not.toHaveBeenCalled()
  })

  it('refuses a paste over 4 MiB at once, keeping what the field held', () => {
    renderDialog()
    paste('{"a": 1}')
    paste(`"${'a'.repeat(MAX_BUNDLE_BYTES)}"`)

    expect(screen.getByRole('alert')).toHaveTextContent(
      'This JSON is over 4 MiB, the most an import can carry.',
    )
    expect(field()).toHaveValue('{"a": 1}')
    expect(api.checkImport).not.toHaveBeenCalled()

    paste('{"a": 2}')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('lists what the JSON holds as Library rows, writes nothing, then imports on the next press', async () => {
    const { onImported } = renderDialog()
    paste(`\n${JSON.stringify(BUNDLE)}\n`)
    press('Import')

    expect(await screen.findByText('A')).toBeInTheDocument()
    expect(screen.getByText('B')).toBeInTheDocument()
    expect(screen.getByText('Most popular')).toBeInTheDocument()
    expect(screen.getByText('Movies')).toBeInTheDocument()
    expect(screen.getByText('Series')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Catalogs · 2' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: /^Collections/ })).not.toBeInTheDocument()
    expect(screen.queryByText(/Choose what Import does/)).not.toBeInTheDocument()
    expect(screen.getByText(/^This JSON holds/).parentElement).toHaveFocus()
    expect(api.checkImport).toHaveBeenCalledWith(3, BUNDLE)
    expect(api.importBundle).not.toHaveBeenCalled()

    press('Import 2 catalogs')
    await waitFor(() => expect(onImported).toHaveBeenCalled())
    expect(api.importBundle).toHaveBeenCalledWith(3, BUNDLE, {}, [])
  })

  it('stops at matches with the field put away, then writes with the choices made on the next press', async () => {
    api.checkImport.mockResolvedValue(MATCHED)
    const { onImported } = renderDialog()
    paste(JSON.stringify(BUNDLE))
    press('Import')

    expect(await screen.findByText('Skip, I already have it')).toBeInTheDocument()
    expect(screen.getByText(/^This JSON holds/)).toHaveTextContent(
      'This JSON holds 2 catalogs, 0 collections and 0 folders.',
    )
    expect(screen.queryByRole('textbox', { name: 'Import JSON' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Validate JSON' })).not.toBeInTheDocument()
    expect(api.importBundle).not.toHaveBeenCalled()

    press('Skip, I already have it')
    press('Import 1 catalog')
    await waitFor(() => expect(onImported).toHaveBeenCalled())
    expect(api.checkImport).toHaveBeenCalledTimes(1)
    expect(api.importBundle).toHaveBeenCalledWith(3, BUNDLE, { c1: 'mine-1' }, [])
  })

  it('moves focus to the summary and its question when the matches appear', async () => {
    api.checkImport.mockResolvedValue(MATCHED)
    renderDialog()
    paste(JSON.stringify(BUNDLE))
    press('Import')

    const question = await screen.findByText(/same filters as a catalog you already have/)
    expect(question.parentElement).toHaveFocus()
  })

  it('names what the next press adds, counting down as matches are skipped', async () => {
    api.checkImport.mockResolvedValue({
      catalogs: [MATCHED.catalogs[0]],
      collections: [{ title: 'Z', folders: [], matched: false, catalogs: [] }],
    })
    renderDialog()
    paste(JSON.stringify(BUNDLE))
    press('Import')
    await screen.findByRole('button', { name: 'Import 1 catalog and 1 collection' })

    press('Skip, I already have it')
    expect(screen.getByRole('button', { name: 'Import 1 collection' })).toBeEnabled()
  })

  it('turns Import off when every catalog is skipped and nothing is left to add', async () => {
    api.checkImport.mockResolvedValue({ catalogs: [MATCHED.catalogs[0]], collections: [] })
    renderDialog()
    paste(JSON.stringify(BUNDLE))
    press('Import')
    await screen.findByRole('button', { name: 'Import 1 catalog' })

    press('Skip, I already have it')
    expect(screen.getByRole('button', { name: 'Nothing to import' })).toBeDisabled()
  })

  it('asks about a collection you already have, and skipping it takes its catalogs out too', async () => {
    api.checkImport.mockResolvedValue(HALLOWEEN)
    const { onImported } = renderDialog()
    paste(JSON.stringify(BUNDLE))
    press('Import')

    expect(await screen.findByRole('group', { name: 'What to do with Halloween' })).toBeInTheDocument()
    expect(screen.getByText('2 folders · Scary, Cosy')).toBeInTheDocument()
    expect(screen.getByRole('group', { name: 'What to do with S' })).toBeInTheDocument()
    expect(screen.queryByText('T')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Import 3 catalogs and 1 collection' })).toBeEnabled()

    fireEvent.click(
      within(screen.getByRole('group', { name: 'What to do with Halloween' })).getByRole('button', {
        name: 'Skip, I already have it',
      }),
    )
    expect(screen.queryByRole('group', { name: 'What to do with S' })).not.toBeInTheDocument()
    expect(screen.getByText('Its catalogs are left out too.')).toBeInTheDocument()

    press('Import 1 catalog')
    await waitFor(() => expect(onImported).toHaveBeenCalled())
    expect(api.importBundle).toHaveBeenCalledWith(3, BUNDLE, {}, [0])
  })

  it('stops for a collection match alone, with no catalog-wide buttons', async () => {
    api.checkImport.mockResolvedValue({
      catalogs: [catalog('c1', 'A')],
      collections: [{ ...HALLOWEEN.collections[0], catalogs: [catalog('s1', 'S'), catalog('s2', 'T')] }],
    })
    renderDialog()
    paste(JSON.stringify(BUNDLE))
    press('Import')

    const question = await screen.findByText(
      'One of these has the same title as a collection you already have. Choose what Import does with each.',
    )
    expect(question.parentElement).toHaveFocus()
    expect(screen.queryByRole('button', { name: 'Import all' })).not.toBeInTheDocument()
    expect(api.importBundle).not.toHaveBeenCalled()
  })

  it('imports a copy of every match unless a choice says otherwise', async () => {
    api.checkImport.mockResolvedValue(MATCHED)
    renderDialog()
    paste(JSON.stringify(BUNDLE))
    press('Import')
    await screen.findByText('Skip, I already have it')
    press('Import 2 catalogs')

    await waitFor(() => expect(api.importBundle).toHaveBeenCalledWith(3, BUNDLE, {}, []))
  })

  it('brings the field back on Edit JSON and drops the matches, so a choice never rides a different bundle', async () => {
    api.checkImport.mockResolvedValue(MATCHED)
    renderDialog()
    paste(JSON.stringify(BUNDLE))
    press('Import')
    await screen.findByText('Skip, I already have it')

    press('Edit JSON')
    expect(field()).toHaveValue(JSON.stringify(BUNDLE))
    expect(field()).toHaveFocus()
    expect(screen.queryByText('Skip, I already have it')).not.toBeInTheDocument()

    press('Import')
    await waitFor(() => expect(api.checkImport).toHaveBeenCalledTimes(2))
  })

  it('names the JSON in a server refusal, moves focus to it, and drops it on the next edit', async () => {
    api.checkImport.mockRejectedValue(new ApiError(400, 'not a bundle'))
    renderDialog()
    paste('{"a":1}')
    press('Import')

    expect(await screen.findByRole('alert')).toHaveTextContent(
      "This JSON isn't a bundle Uno can import.not a bundle",
    )
    expect(screen.getByRole('alert')).toHaveFocus()
    expect(api.importBundle).not.toHaveBeenCalled()

    fireEvent.change(field(), { target: { value: '{"a":2}' } })
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('shows a failed write and keeps the review', async () => {
    api.importBundle.mockRejectedValue(new ApiError(500, 'disk full'))
    renderDialog()
    paste(JSON.stringify(BUNDLE))
    press('Import')
    await screen.findByRole('button', { name: 'Import 2 catalogs' })
    press('Import 2 catalogs')

    expect(await screen.findByRole('alert')).toHaveTextContent("Couldn't import. Nothing was added.disk full")
    expect(screen.getByRole('alert')).toHaveFocus()
    expect(screen.getByText('A')).toBeInTheDocument()
  })
})
