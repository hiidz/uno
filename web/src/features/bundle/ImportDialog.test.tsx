// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/api/http'
import type { ImportCheck } from '@/api'
import { ImportDialog } from './ImportDialog'
import { MAX_BUNDLE_BYTES } from './text'

const api = vi.hoisted(() => ({ checkImport: vi.fn(), importBundle: vi.fn() }))
vi.mock('@/api', async () => ({ queryKeys: (await import('@/api/keys')).queryKeys, ...api }))
vi.mock('@/api/client', () => ({ apiFetch: vi.fn() }))

const BUNDLE = { format: 'uno-bundle', catalogs: [{ name: 'A' }, { name: 'B' }] }
const CHECK: ImportCheck = { catalogs: 2, collections: 0, folders: 0, matches: [] }
const MATCHED: ImportCheck = {
  ...CHECK,
  matches: [
    {
      key: 'c1',
      name: 'A',
      type: 'movie',
      scope: 'listed',
      collection: '',
      existing: [{ id: 'mine-1', name: 'My A' }],
    },
  ],
}
const RESULT = { catalogs: [], collections: [] }

function renderDialog() {
  const onImported = vi.fn()
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ImportDialog open profileIndex={3} onClose={vi.fn()} onImported={onImported} />
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

    expect(screen.getByRole('alert')).toHaveTextContent(/^Pasted JSON isn't valid: /)
    expect(field()).toHaveValue('{nope')
    expect(api.checkImport).not.toHaveBeenCalled()

    fireEvent.change(field(), { target: { value: '{' } })
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('refuses invalid JSON on Import without sending it', () => {
    renderDialog()
    paste('{nope')
    press('Import')

    expect(screen.getByRole('alert')).toHaveTextContent(/^Pasted JSON isn't valid: /)
    expect(api.checkImport).not.toHaveBeenCalled()
  })

  it('refuses text over 4 MiB without sending it', () => {
    renderDialog()
    paste(`"${'a'.repeat(MAX_BUNDLE_BYTES)}"`)
    press('Validate JSON')

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Pasted JSON is over 4 MiB, the most an import can carry.',
    )
    expect(api.checkImport).not.toHaveBeenCalled()
  })

  it('imports in one press when nothing matches: the check, then the write', async () => {
    const { onImported } = renderDialog()
    paste(`\n${JSON.stringify(BUNDLE)}\n`)
    press('Import')

    await waitFor(() => expect(onImported).toHaveBeenCalled())
    expect(api.checkImport).toHaveBeenCalledWith(3, BUNDLE)
    expect(api.importBundle).toHaveBeenCalledWith(3, BUNDLE, {})
  })

  it('stops at matches, then writes with the choices made on the next press', async () => {
    api.checkImport.mockResolvedValue(MATCHED)
    const { onImported } = renderDialog()
    paste(JSON.stringify(BUNDLE))
    press('Import')

    expect(await screen.findByText('Skip, I already have it')).toBeInTheDocument()
    expect(screen.getByText(/Pasted JSON/)).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: 'Import JSON' })).toBeInTheDocument()
    expect(api.importBundle).not.toHaveBeenCalled()

    press('Skip, I already have it')
    press('Import')
    await waitFor(() => expect(onImported).toHaveBeenCalled())
    expect(api.checkImport).toHaveBeenCalledTimes(1)
    expect(api.importBundle).toHaveBeenCalledWith(3, BUNDLE, { c1: 'mine-1' })
  })

  it('imports a copy of every match unless a choice says otherwise', async () => {
    api.checkImport.mockResolvedValue(MATCHED)
    renderDialog()
    paste(JSON.stringify(BUNDLE))
    press('Import')
    await screen.findByText('Skip, I already have it')
    press('Import')

    await waitFor(() => expect(api.importBundle).toHaveBeenCalledWith(3, BUNDLE, {}))
  })

  it('drops the matches when the text is edited, so a choice never rides a different bundle', async () => {
    api.checkImport.mockResolvedValue(MATCHED)
    renderDialog()
    paste(JSON.stringify(BUNDLE))
    press('Import')
    await screen.findByText('Skip, I already have it')

    fireEvent.change(field(), { target: { value: '{"format":"other"}' } })
    expect(screen.queryByText('Skip, I already have it')).not.toBeInTheDocument()

    press('Import')
    await waitFor(() => expect(api.checkImport).toHaveBeenCalledTimes(2))
  })

  it('names the JSON in a server refusal, and drops it on the next edit', async () => {
    api.checkImport.mockRejectedValue(new ApiError(400, 'not a bundle'))
    renderDialog()
    paste('{"a":1}')
    press('Import')

    expect(await screen.findByRole('alert')).toHaveTextContent(
      "This JSON can't be imported: not a bundle",
    )
    expect(api.importBundle).not.toHaveBeenCalled()

    fireEvent.change(field(), { target: { value: '{"a":2}' } })
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('shows a failed write and keeps the field', async () => {
    api.importBundle.mockRejectedValue(new ApiError(500, 'disk full'))
    renderDialog()
    paste(JSON.stringify(BUNDLE))
    press('Import')

    expect(await screen.findByRole('alert')).toHaveTextContent("Couldn't import: disk full")
    expect(field()).toHaveValue(JSON.stringify(BUNDLE))
  })
})
