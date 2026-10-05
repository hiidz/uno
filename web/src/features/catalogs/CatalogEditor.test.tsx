// @vitest-environment jsdom
import type { ComponentProps } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { catalog } from '@/test/fixtures'
import { CatalogEditor } from './CatalogEditor'
import { emptyForm, formFromCatalog } from './catalogForm'

// Every request the editor's pickers and preview make stays pending: these
// tests are about the form, not what TMDB answers.
vi.mock('@/api/client', () => ({ apiFetch: vi.fn(() => new Promise(() => {})) }))

function renderEditor(props: Partial<ComponentProps<typeof CatalogEditor>> = {}) {
  const handlers = { onSave: vi.fn(), onRequestClose: vi.fn(), onDirtyChange: vi.fn() }
  render(
    <QueryClientProvider client={new QueryClient()}>
      <CatalogEditor
        initial={emptyForm()}
        genres={{ movie: [], tv: [] }}
        certifications={{ movie: {}, tv: {} }}
        countryNames={new Map()}
        languages={[]}
        saving={false}
        serverError={null}
        {...handlers}
        {...props}
      />
    </QueryClientProvider>,
  )
  return handlers
}

const nameInput = () => screen.getByLabelText('Name')
const save = () => fireEvent.click(screen.getByRole('button', { name: 'Save' }))

describe('CatalogEditor', () => {
  it('shows what needs fixing instead of saving a catalog with no name', () => {
    const { onSave } = renderEditor()
    save()
    expect(onSave).not.toHaveBeenCalled()
    expect(screen.getByText('Give this catalog a name.')).toBeInTheDocument()
    expect(nameInput()).toHaveFocus()
  })

  it('saves the form once it is valid, reporting it dirty until then', () => {
    const { onSave, onDirtyChange } = renderEditor()
    fireEvent.change(nameInput(), { target: { value: 'Trending Sci-Fi' } })
    expect(onDirtyChange).toHaveBeenLastCalledWith(true)
    expect(screen.getByText('Unsaved changes')).toBeInTheDocument()

    save()
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ name: 'Trending Sci-Fi', type: 'movie' }))
  })

  it('shows a rejected save as an alert', () => {
    renderEditor({ initial: formFromCatalog(catalog()), serverError: 'name already taken' })
    expect(screen.getByRole('alert')).toHaveTextContent("Couldn't save this catalog: name already taken")
  })

  it('carries the step and stickers the pane hands it, and saves without asking', () => {
    const onClick = vi.fn()
    const { onSave } = renderEditor({
      initial: formFromCatalog(catalog({ name: 'Row' })),
      sharingStep: { label: 'Publish…', waiting: null, onClick },
      sharingBadges: <span>Published sticker</span>,
    })
    // One button, in the save bar.
    fireEvent.click(screen.getByRole('button', { name: 'Publish…' }))
    expect(onClick).toHaveBeenCalled()
    // The sign shows it from `sm`; below, the body's first line does.
    expect(screen.getAllByText('Published sticker')).toHaveLength(2)
    for (const kind of screen.getAllByText('Movies', { selector: '.stk' })) expect(kind).toHaveClass('stk-catalog')
    fireEvent.change(nameInput(), { target: { value: 'Renamed' } })
    save()
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ name: 'Renamed' }))
  })

  it('opens Recent on 90 days, and offers Upcoming and Dates beside it', () => {
    renderEditor({ initial: formFromCatalog(catalog({ name: 'Row' })) })
    fireEvent.click(screen.getByRole('button', { name: /^Release date/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Recent' }))
    expect(screen.getByRole('button', { name: '90 days' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: '10 years' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Upcoming' }))
    expect(screen.getByText(/onward, updated daily/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Recent' }))
    expect(screen.getByRole('button', { name: '90 days' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('keeps a stored window no preset matches as a chip of its own', () => {
    const initial = formFromCatalog(catalog({ name: 'Row', params: '{"released_within_days":1095}' }))
    renderEditor({ initial })
    fireEvent.click(screen.getByRole('button', { name: /^Release date/ }))
    expect(screen.getByRole('button', { name: '3 years' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('shuffles from the Order section, and says so in its summary', () => {
    renderEditor({ initial: formFromCatalog(catalog({ name: 'Row' })) })
    fireEvent.click(screen.getByRole('button', { name: /^Order/ }))
    fireEvent.click(screen.getByRole('button', { name: 'On' }))
    expect(screen.getByRole('button', { name: /^Order.*· shuffled/ })).toBeInTheDocument()
    expect(screen.getByText('New set each time')).toBeInTheDocument()
  })

  it('saves what the Order, Genres and Original language controls set', () => {
    const { onSave } = renderEditor({
      initial: formFromCatalog(catalog({ name: 'Row' })),
      genres: { movie: [{ id: 27, name: 'Horror' }], tv: [] },
      languages: [{ iso_639_1: 'it', english_name: 'Italian', name: 'Italiano' }],
    })
    fireEvent.click(screen.getByRole('button', { name: /^Order/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Low to high' }))
    fireEvent.click(screen.getByRole('button', { name: /^Genres/ }))
    fireEvent.click(screen.getByRole('button', { name: /Horror/ }))
    fireEvent.click(screen.getByRole('button', { name: /^Original language/ }))
    fireEvent.change(screen.getByRole('combobox', { name: 'Original language' }), { target: { value: 'it' } })
    save()
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        params: expect.objectContaining({ sort_by: 'popularity.asc', with_genres: '27', with_original_language: 'it' }),
      }),
    )
  })

  it('names the section to fix in place of results', () => {
    const initial = formFromCatalog(catalog({ name: 'Row', params: '{"with_collection":""}' }))
    renderEditor({ initial: { ...initial, sourceMode: 'collection' } })
    fireEvent.click(screen.getByRole('button', { name: 'Run preview' }))
    expect(screen.getByText('Fix TMDB collection first.')).toBeInTheDocument()
  })

  it('saves a scoped catalog as Done in a collection, with no Scope setting', () => {
    renderEditor({ initial: formFromCatalog(catalog({ name: 'Row', collection_id: 'col1' })), saveLabel: 'Done' })
    expect(screen.queryByText('Scope')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Move to library' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Done' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Close' })).toBeInTheDocument()
  })
})
