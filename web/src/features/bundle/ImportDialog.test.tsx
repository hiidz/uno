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

function paste(text: string) {
  fireEvent.click(screen.getByRole('button', { name: 'Paste text' }))
  fireEvent.change(screen.getByRole('textbox', { name: 'Export text' }), { target: { value: text } })
}

beforeEach(() => {
  api.checkImport.mockReset()
  api.importBundle.mockReset()
  api.checkImport.mockResolvedValue(CHECK)
  api.importBundle.mockResolvedValue(RESULT)
})

describe('ImportDialog paste', () => {
  it('shows one input at a time, File first', () => {
    renderDialog()
    expect(screen.getByText('Choose a file…')).toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Paste text' }))
    expect(screen.getByRole('textbox', { name: 'Export text' })).toBeInTheDocument()
    expect(screen.queryByText('Choose a file…')).not.toBeInTheDocument()
  })

  it('keeps Check text off while the box is blank', () => {
    renderDialog()
    paste('   ')
    expect(screen.getByRole('button', { name: 'Check text' })).toBeDisabled()
  })

  it('refuses invalid JSON in place, keeps the text, sends nothing, and clears on the next edit', () => {
    renderDialog()
    paste('{nope')
    fireEvent.click(screen.getByRole('button', { name: 'Check text' }))

    expect(screen.getByRole('alert')).toHaveTextContent(/^Pasted text isn't valid JSON: /)
    expect(screen.getByRole('textbox', { name: 'Export text' })).toHaveValue('{nope')
    expect(api.checkImport).not.toHaveBeenCalled()

    fireEvent.change(screen.getByRole('textbox', { name: 'Export text' }), { target: { value: '{' } })
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('refuses text over 4 MiB without sending it', () => {
    renderDialog()
    paste(`"${'a'.repeat(MAX_BUNDLE_BYTES)}"`)
    fireEvent.click(screen.getByRole('button', { name: 'Check text' }))

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Pasted text is over 4 MiB, the most an import can carry.',
    )
    expect(api.checkImport).not.toHaveBeenCalled()
  })

  it('checks valid text, reviews it as Pasted text, and imports the same payload a file does', async () => {
    const { onImported } = renderDialog()
    paste(`\n${JSON.stringify(BUNDLE)}\n`)
    fireEvent.click(screen.getByRole('button', { name: 'Check text' }))

    expect(await screen.findByText('Pasted text')).toBeInTheDocument()
    expect(screen.getByText(/holds 2 catalogs/)).toBeInTheDocument()
    expect(api.checkImport).toHaveBeenCalledWith(3, BUNDLE)

    fireEvent.click(screen.getByRole('button', { name: 'Import' }))
    await waitFor(() => expect(onImported).toHaveBeenCalled())
    expect(api.importBundle).toHaveBeenCalledWith(3, BUNDLE, {})
  })

  it('names the text in a server refusal, and drops it when the mode switches', async () => {
    api.checkImport.mockRejectedValue(new ApiError(400, 'not a bundle'))
    renderDialog()
    paste('{"a":1}')
    fireEvent.click(screen.getByRole('button', { name: 'Check text' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      "This text can't be imported: not a bundle",
    )

    fireEvent.click(screen.getByRole('button', { name: 'File' }))
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})

describe('ImportDialog file', () => {
  it('reads a picked file through the same check and review', async () => {
    renderDialog()
    const file = new File([JSON.stringify(BUNDLE)], 'mine.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', { value: () => Promise.resolve(JSON.stringify(BUNDLE)) })
    fireEvent.change(document.querySelector('input[type=file]')!, { target: { files: [file] } })

    expect(await screen.findByText('mine.json')).toBeInTheDocument()
    expect(api.checkImport).toHaveBeenCalledWith(3, BUNDLE)
  })

  it('refuses a file over 4 MiB before reading it', async () => {
    renderDialog()
    const file = new File(['{}'], 'big.json')
    Object.defineProperty(file, 'size', { value: MAX_BUNDLE_BYTES + 1 })
    fireEvent.change(document.querySelector('input[type=file]')!, { target: { files: [file] } })

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'big.json is over 4 MiB, the most an import can carry.',
    )
    expect(api.checkImport).not.toHaveBeenCalled()
  })

  it('names the file in a server refusal', async () => {
    api.checkImport.mockRejectedValue(new ApiError(400, 'not a bundle'))
    renderDialog()
    const file = new File(['{"a":1}'], 'x.json')
    Object.defineProperty(file, 'text', { value: () => Promise.resolve('{"a":1}') })
    fireEvent.change(document.querySelector('input[type=file]')!, { target: { files: [file] } })

    expect(await screen.findByRole('alert')).toHaveTextContent(
      "This file can't be imported: not a bundle",
    )
  })
})
