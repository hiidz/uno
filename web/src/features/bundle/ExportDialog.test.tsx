// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { catalog, collection } from '@/test/fixtures'
import { ExportDialog } from './ExportDialog'
import { downloadJSON } from './download'
import { bundleText } from './text'

const api = vi.hoisted(() => ({ exportBundle: vi.fn() }))
vi.mock('@/api', () => api)
vi.mock('./download', () => ({
  downloadJSON: vi.fn(),
  exportFilename: () => 'uno-export-test.json',
}))

const BUNDLE = { format: 'uno-bundle', catalogs: [{ name: 'Popular' }] }
const catalogs = [catalog({ id: 'c1', name: 'Popular' }), catalog({ id: 'c2', name: 'Quiet' })]
const collections = [{ ...collection({ id: 'k1', title: 'Shelf' }), folders: [] }]

function setClipboard(clipboard: unknown) {
  Object.defineProperty(navigator, 'clipboard', { value: clipboard, configurable: true })
}

function renderDialog(preselected: string | null = 'c1') {
  const onClose = vi.fn()
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ExportDialog
        open
        profileIndex={2}
        catalogs={catalogs}
        collections={collections}
        preselected={preselected}
        onClose={onClose}
      />
    </QueryClientProvider>,
  )
  return { onClose }
}

beforeEach(() => {
  // jsdom does not lay out, so it has no scrollIntoView.
  Element.prototype.scrollIntoView = vi.fn()
  api.exportBundle.mockReset()
  api.exportBundle.mockResolvedValue(BUNDLE)
  vi.mocked(downloadJSON).mockReset()
})

afterEach(() => {
  setClipboard(undefined)
})

describe('ExportDialog', () => {
  it('copies the pretty JSON of the ticked ids, stays open, and reads Copied', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    setClipboard({ writeText })
    const { onClose } = renderDialog()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Quiet' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Shelf' }))
    fireEvent.click(screen.getByRole('button', { name: 'Copy text' }))

    expect(await screen.findByRole('button', { name: 'Copied' })).toBeInTheDocument()
    expect(api.exportBundle).toHaveBeenCalledWith(2, {
      catalog_ids: ['c1', 'c2'],
      collection_ids: ['k1'],
    })
    expect(writeText).toHaveBeenCalledWith(bundleText(BUNDLE))
    expect(downloadJSON).not.toHaveBeenCalled()
    expect(onClose).not.toHaveBeenCalled()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()

    // The label reverts on its own.
    expect(await screen.findByRole('button', { name: 'Copy text' }, { timeout: 3000 })).toBeEnabled()
  })

  it('shows the export text in a read-only box when the copy fails, and stays open', async () => {
    setClipboard({ writeText: vi.fn().mockRejectedValue(new Error('denied')) })
    const { onClose } = renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Copy text' }))

    const box = await screen.findByRole('textbox', { name: 'Export text' })
    expect(box).toHaveValue(bundleText(BUNDLE))
    expect(box).toHaveAttribute('readonly')
    expect(screen.getByRole('alert')).toHaveTextContent("Couldn't copy. Copy the text below.")
    expect(screen.queryByRole('button', { name: 'Copied' })).not.toBeInTheDocument()
    expect(onClose).not.toHaveBeenCalled()
  })

  it('shows the box when the browser has no clipboard', async () => {
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Copy text' }))
    expect(await screen.findByRole('textbox', { name: 'Export text' })).toBeInTheDocument()
  })

  it('downloads the file and closes on Download', async () => {
    const { onClose } = renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Download' }))

    await waitFor(() => expect(onClose).toHaveBeenCalled())
    expect(downloadJSON).toHaveBeenCalledWith(BUNDLE, 'uno-export-test.json')
  })

  it('shows a failed export in place and copies nothing', async () => {
    const writeText = vi.fn()
    setClipboard({ writeText })
    api.exportBundle.mockRejectedValue(new Error('server down'))
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Copy text' }))

    expect(await screen.findByRole('alert')).toHaveTextContent("Couldn't export: server down")
    expect(writeText).not.toHaveBeenCalled()
  })

  it('clears the copy-failure box when the ticks change', async () => {
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Copy text' }))
    await screen.findByRole('textbox', { name: 'Export text' })

    fireEvent.click(screen.getByRole('checkbox', { name: 'Popular' }))
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy text' })).toBeDisabled()
  })

  it('disables both buttons while nothing is ticked', () => {
    renderDialog(null)
    expect(screen.getByRole('button', { name: 'Copy text' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Download' })).toBeDisabled()
  })
})
