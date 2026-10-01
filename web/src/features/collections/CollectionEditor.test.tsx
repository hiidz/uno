// @vitest-environment jsdom
import type { ComponentProps } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { catalog, collection, folder } from '@/test/fixtures'
import { CollectionEditor } from './CollectionEditor'
import { emptyCollectionForm, formFromCollection } from './collectionForm'
import { accessibleIDs, buildRefOptions, indexRefOptions } from './refs'

// Every request the nested pickers and previews make stays pending: these
// tests are about the form, not what TMDB answers.
vi.mock('@/api/client', () => ({ apiFetch: vi.fn(() => new Promise(() => {})) }))

// A folder row counts the home screen among a catalog's places; nothing here
// is on it.
vi.mock('@/features/home/useHomeSelection', () => ({ useHomeSelection: () => ({ hasCatalog: () => false }) }))

const library = [catalog({ id: 'c1', name: 'Late Night' })]
const genreLookups = { movie: new Map<number, string>(), tv: new Map<number, string>() }
const options = buildRefOptions(library, genreLookups)

const saved = collection({
  title: 'Weekend',
  folders: [folder({ id: 'f1', title: 'Horror', refs: [{ catalog_id: 'c1', genre: '' }] })],
  catalogs: library,
})

function renderEditor(props: Partial<ComponentProps<typeof CollectionEditor>> = {}) {
  const handlers = { onSave: vi.fn(), onRequestClose: vi.fn(), onDirtyChange: vi.fn() }
  render(
    <QueryClientProvider client={new QueryClient()}>
      <CollectionEditor
        initial={emptyCollectionForm()}
        collectionID={saved.id}
        options={options}
        optionByID={indexRefOptions(options)}
        accessibleIDs={accessibleIDs(options)}
        saving={false}
        serverError={null}
        initialCatalogs={[]}
        genres={{ movie: [], tv: [] }}
        genreLookups={genreLookups}
        certifications={{ movie: {}, tv: {} }}
        countryNames={new Map()}
        languages={[]}
        usedInFolders={() => 0}
        {...handlers}
        {...props}
      />
    </QueryClientProvider>,
  )
  return handlers
}

const titleInput = () => screen.getByLabelText('Title')
const save = () => fireEvent.click(screen.getByRole('button', { name: 'Save collection' }))

describe('CollectionEditor', () => {
  it('shows what needs fixing instead of saving a collection with no title', () => {
    const { onSave } = renderEditor()
    save()
    expect(onSave).not.toHaveBeenCalled()
    expect(screen.getByText('Give this collection a title.')).toBeInTheDocument()
  })

  it('shows what needs fixing instead of saving a folder with no title', () => {
    const { onSave } = renderEditor({ initial: formFromCollection(saved) })
    fireEvent.click(screen.getByRole('button', { name: 'Add folder' }))
    save()
    expect(onSave).not.toHaveBeenCalled()
    expect(screen.getByText('Every folder needs a title.')).toBeInTheDocument()
  })

  it('saves the finished payload, keeping each folder and its catalogs', () => {
    const { onSave, onDirtyChange } = renderEditor({
      initial: formFromCollection(saved),
      initialCatalogs: library,
    })
    fireEvent.change(titleInput(), { target: { value: '  Weekend nights ' } })
    expect(onDirtyChange).toHaveBeenLastCalledWith(true)

    save()
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        title: 'Weekend nights',
        folders: [expect.objectContaining({ id: 'f1', title: 'Horror', catalogs: [{ catalog_id: 'c1' }] })],
      }),
    )
  })

  it('shows the Sharing row the pane hands it, and saves without asking', () => {
    const { onSave } = renderEditor({
      initial: formFromCollection(saved),
      initialCatalogs: library,
      sharingRow: <p>Sharing slot</p>,
    })
    expect(screen.getByText('Sharing slot')).toBeInTheDocument()
    fireEvent.change(titleInput(), { target: { value: 'Mine now' } })
    save()
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ title: 'Mine now' }))
  })

  it('asks to delete the collection, whatever Nuvio holds', () => {
    const onDelete = vi.fn()
    renderEditor({ initial: formFromCollection(saved), onDelete })
    fireEvent.click(screen.getByRole('button', { name: 'Delete Weekend' }))
    expect(onDelete).toHaveBeenCalled()
  })
})
