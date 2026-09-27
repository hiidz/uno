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

  describe('on a linked copy', () => {
    const linked = { initial: formFromCatalog(catalog({ name: 'Row', linked: true })), linked: true }

    it('asks before a save that would unlink it, and saves once confirmed', () => {
      const { onSave } = renderEditor(linked)
      expect(screen.getByRole('note')).toHaveTextContent('Editing unlinks it from the community catalog')
      fireEvent.change(nameInput(), { target: { value: 'Renamed' } })

      save()
      expect(onSave).not.toHaveBeenCalled()
      fireEvent.click(screen.getByRole('button', { name: 'Save and unlink' }))
      expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ name: 'Renamed' }))
    })

    it('keeps editing without saving when the unlink is declined', () => {
      const { onSave } = renderEditor(linked)
      fireEvent.change(nameInput(), { target: { value: 'Renamed' } })
      save()
      fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }))
      expect(onSave).not.toHaveBeenCalled()
      expect(nameInput()).toHaveValue('Renamed')
    })

    it('saves a sharing change without asking, since it keeps the link', () => {
      const { onSave } = renderEditor(linked)
      fireEvent.click(screen.getByRole('switch'))
      save()
      expect(screen.queryByRole('button', { name: 'Save and unlink' })).not.toBeInTheDocument()
      expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ isPublic: true }))
    })
  })
})
