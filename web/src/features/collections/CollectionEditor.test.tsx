// @vitest-environment jsdom
import type { ComponentProps } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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

const library = [
  catalog({ id: 'c1', name: 'Late Night' }),
  catalog({ id: 'c2', name: 'Giallo' }),
  catalog({ id: 'c3', name: 'Noir', type: 'series' }),
]
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
const save = () => fireEvent.click(screen.getByRole('button', { name: 'Save' }))

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

  it('carries the step the pane hands it, and saves without asking', () => {
    const { onSave } = renderEditor({
      initial: formFromCollection(saved),
      initialCatalogs: library,
      sharingStep: { label: 'Publish…', waiting: null, onClick: vi.fn() },
    })
    expect(screen.getAllByRole('button', { name: 'Publish…' })).toHaveLength(1)
    fireEvent.change(titleInput(), { target: { value: 'Mine now' } })
    save()
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ title: 'Mine now' }))
  })

  it('folds the collection’s appearance into one shelf after its folders', () => {
    renderEditor({ initial: formFromCollection(saved), initialCatalogs: library })
    const shelf = screen.getByRole('button', { name: /^Collection Appearance\s*Rows · glow on$/ })
    const folders = screen.getByRole('heading', { name: 'Folders' })
    expect(folders.compareDocumentPosition(shelf) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    fireEvent.click(shelf)
    fireEvent.click(within(screen.getByRole('group', { name: 'Focus glow' })).getByRole('button', { name: 'Off' }))
    expect(screen.getByRole('button', { name: /^Collection Appearance\s*Rows$/ })).toBeInTheDocument()
  })

  it('heads the open folder with where it sits, and states each catalog’s kind', () => {
    renderEditor({ initial: formFromCollection(saved), initialCatalogs: library })
    expect(screen.getByText('Folder 1')).toBeInTheDocument()
    expect(screen.getByText('Movies', { selector: '.stk' })).toHaveClass('stk-catalog')
    expect(screen.getByRole('combobox', { name: 'Genre' })).toBeInTheDocument()
  })

  it('asks before the nested editor drops its unsaved edits, and stages them with Done', () => {
    const scoped = catalog({ id: 's1', name: 'Scoped', collection_id: saved.id })
    const withScoped = { ...saved, folders: [folder({ id: 'f1', title: 'Horror', refs: [{ catalog_id: 's1', genre: '' }] })] }
    const { onDirtyChange } = renderEditor({ initial: formFromCollection(withScoped), initialCatalogs: [scoped] })
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    const nested = () => screen.getByRole('dialog', { name: 'Edit Scoped' })
    fireEvent.change(within(nested()).getByLabelText('Name'), { target: { value: 'Scoped, renamed' } })

    fireEvent.click(within(nested()).getByRole('button', { name: 'Close' }))
    expect(screen.getByRole('dialog', { name: 'Discard unsaved changes?' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }))
    expect(within(nested()).getByLabelText('Name')).toHaveValue('Scoped, renamed')

    fireEvent.click(within(nested()).getByRole('button', { name: 'Done' }))
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(onDirtyChange).toHaveBeenLastCalledWith(true)
  })

  it('asks to delete the collection, whatever Nuvio holds', () => {
    const onDelete = vi.fn()
    renderEditor({ initial: formFromCollection(saved), onDelete })
    fireEvent.click(screen.getByRole('button', { name: 'Delete Weekend' }))
    expect(onDelete).toHaveBeenCalled()
  })
  it('names a new catalog inside the collection, opens it one level down, and stages it with Done', () => {
    const { onSave } = renderEditor({ initial: formFromCollection(saved), initialCatalogs: library })
    fireEvent.click(screen.getByRole('button', { name: 'New catalog' }))
    const naming = screen.getByRole('dialog')
    fireEvent.change(within(naming).getByLabelText('Name'), { target: { value: 'Giallo' } })
    fireEvent.submit(within(naming).getByLabelText('Name').closest('form')!)
    const nested = screen.getByRole('dialog', { name: 'Edit Giallo' })
    expect(within(nested).getByText('Only in this collection')).toBeInTheDocument()
    fireEvent.click(within(nested).getByRole('button', { name: 'Done' }))
    save()
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        folders: [expect.objectContaining({ catalogs: expect.arrayContaining([expect.objectContaining({ new: expect.objectContaining({ name: 'Giallo' }) })]) })],
      }),
    )
  })

  describe('the Add catalogs dropdown', () => {
    const open = () => fireEvent.click(screen.getByRole('button', { name: 'Add catalogs' }))
    const dropdown = () => screen.getByRole('dialog', { name: 'Add catalogs to this folder' })
    const folderRows = () => screen.getAllByRole('listitem').filter((row) => row.querySelector('select'))

    it('lists only what the folder lacks, with each catalog’s kind, and adds the ticked ones in library order', () => {
      const { onSave } = renderEditor({ initial: formFromCollection(saved), initialCatalogs: library })
      open()
      expect(within(dropdown()).queryByLabelText(/Late Night/)).toBeNull()
      expect(within(dropdown()).getByText('Series')).toBeInTheDocument()
      // Ticked newest-first: the folder still takes them in library order.
      fireEvent.click(within(dropdown()).getByLabelText(/Noir/))
      fireEvent.click(within(dropdown()).getByLabelText(/Giallo/))
      fireEvent.click(within(dropdown()).getByRole('button', { name: 'Add 2 catalogs' }))
      expect(screen.queryByRole('dialog')).toBeNull()
      expect(folderRows()).toHaveLength(3)
      save()
      expect(onSave).toHaveBeenCalledWith(
        expect.objectContaining({
          folders: [
            expect.objectContaining({
              catalogs: [{ catalog_id: 'c1' }, { catalog_id: 'c2' }, { catalog_id: 'c3' }],
            }),
          ],
        }),
      )
    })

    it('adds nothing on Cancel or Escape', async () => {
      renderEditor({ initial: formFromCollection(saved), initialCatalogs: library })
      open()
      fireEvent.click(within(dropdown()).getByLabelText(/Giallo/))
      fireEvent.click(within(dropdown()).getByRole('button', { name: 'Cancel' }))
      expect(screen.queryByRole('dialog')).toBeNull()

      open()
      expect(within(dropdown()).getByLabelText(/Giallo/)).not.toBeChecked()
      fireEvent.keyDown(dropdown(), { key: 'Escape' })
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(folderRows()).toHaveLength(1)
    })

    it('offers a new catalog named for a search that matches nothing', () => {
      renderEditor({ initial: formFromCollection(saved), initialCatalogs: library })
      open()
      fireEvent.change(within(dropdown()).getByRole('searchbox'), { target: { value: 'Slasher' } })
      fireEvent.click(within(dropdown()).getByRole('button', { name: 'New catalog “Slasher”' }))
      expect(screen.getByRole('dialog', { name: 'New catalog' })).toBeInTheDocument()
      expect(screen.getByLabelText('Name')).toHaveValue('Slasher')
    })
  })

  it('saves a renamed folder and a catalog widened back to all genres', () => {
    const narrowed = {
      ...saved,
      folders: [folder({ id: 'f1', title: 'Horror', refs: [{ catalog_id: 'c1', genre: 'Horror' }] })],
    }
    const { onSave } = renderEditor({ initial: formFromCollection(narrowed), initialCatalogs: library })
    fireEvent.change(screen.getByLabelText('Title of folder 1'), { target: { value: 'Frights' } })
    fireEvent.change(screen.getByRole('combobox', { name: 'Genre' }), { target: { value: '' } })
    save()
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        folders: [expect.objectContaining({ id: 'f1', title: 'Frights', catalogs: [{ catalog_id: 'c1' }] })],
      }),
    )
  })

  it('reorders a folder from the arrows under its tile', () => {
    const two = {
      ...saved,
      folders: [
        folder({ id: 'f1', title: 'Horror', refs: [{ catalog_id: 'c1', genre: '' }] }),
        folder({ id: 'f2', title: 'Romance', refs: [{ catalog_id: 'c2', genre: '' }] }),
      ],
    }
    const { onSave } = renderEditor({ initial: formFromCollection(two), initialCatalogs: library })
    expect(screen.getByRole('button', { name: /^Move .* left, already first$/ })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: /^Move .* right$/ }))
    expect(screen.getByRole('button', { name: /^Move .* right, already last$/ })).toBeDisabled()
    save()
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        folders: [expect.objectContaining({ id: 'f2' }), expect.objectContaining({ id: 'f1' })],
      }),
    )
  })
})
