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

  it('shows the Sharing row and stickers the pane hands it, and saves without asking', () => {
    const { onSave } = renderEditor({
      initial: formFromCatalog(catalog({ name: 'Row' })),
      sharingRow: <p>Sharing slot</p>,
      sharingBadges: <span>Published sticker</span>,
    })
    expect(screen.getByText('Sharing slot')).toBeInTheDocument()
    // The sign shows it from `sm`; below, the body's first line does.
    expect(screen.getAllByText('Published sticker')).toHaveLength(2)
    fireEvent.change(nameInput(), { target: { value: 'Renamed' } })
    save()
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ name: 'Renamed' }))
  })
})
