// @vitest-environment jsdom
import type { ComponentProps } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { apiFetch } from '@/api/client'
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
  const handlers = { onSave: vi.fn(), onRequestClose: vi.fn(), onDirtyChange: vi.fn(), onCopyToLibrary: vi.fn(() => Promise.resolve()) }
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

  it('heads the open folder with its title, and states each catalog’s kind', () => {
    renderEditor({ initial: formFromCollection(saved), initialCatalogs: library })
    expect(screen.getByRole('heading', { name: 'Horror' })).toBeInTheDocument()
    expect(screen.getByText('Movies', { selector: '.stk' })).toHaveClass('stk-catalog')
    expect(screen.getByRole('button', { name: 'Split by genre' })).toBeInTheDocument()
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
    fireEvent.click(screen.getByRole('button', { name: 'Add catalogs' }))
    fireEvent.click(within(screen.getByRole('dialog', { name: 'Add catalogs to this folder' })).getByRole('button', { name: 'New catalog' }))
    const naming = screen.getByRole('dialog')
    fireEvent.change(within(naming).getByLabelText('Name'), { target: { value: 'Giallo' } })
    fireEvent.submit(within(naming).getByLabelText('Name').closest('form')!)
    const nested = screen.getByRole('dialog', { name: 'Edit Giallo' })
    fireEvent.click(within(nested).getByRole('button', { name: 'Done' }))
    save()
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        folders: [expect.objectContaining({ catalogs: expect.arrayContaining([expect.objectContaining({ new: expect.objectContaining({ name: 'Giallo' }) })]) })],
      }),
    )
  })

  describe('Copy into library', () => {
    const scoped = catalog({ id: 's1', name: 'Scoped', params: '{"with_genres":"27"}', collection_id: saved.id })
    const withScoped = {
      ...saved,
      folders: [folder({ id: 'f1', title: 'Horror', refs: [{ catalog_id: 's1', genre: '' }, { catalog_id: 'c1', genre: '' }] })],
    }
    const openMenu = (name: string) =>
      fireEvent.pointerDown(screen.getByRole('button', { name: `More for ${name}` }), { button: 0, ctrlKey: false, pointerType: 'mouse' })

    it('writes a library copy of the catalog as staged, at once, and says so beside the row', async () => {
      const onCopyToLibrary = vi.fn(() => Promise.resolve())
      const { onSave } = renderEditor({ initial: formFromCollection(withScoped), initialCatalogs: [scoped, ...library], onCopyToLibrary })
      fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
      const nested = screen.getByRole('dialog', { name: 'Edit Scoped' })
      fireEvent.change(within(nested).getByLabelText('Name'), { target: { value: 'Staged name' } })
      fireEvent.click(within(nested).getByRole('button', { name: 'Done' }))

      openMenu('Staged name')
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Copy into library' }))
      expect(await screen.findByText('Copied into your library')).toBeInTheDocument()
      expect(onCopyToLibrary).toHaveBeenCalledWith({
        type: 'movie',
        name: 'Staged name',
        provider: 'tmdb',
        params: expect.stringContaining('"with_genres":"27"'),
      })
      expect(onSave).not.toHaveBeenCalled()
    })

    it('says why a copy failed', async () => {
      const onCopyToLibrary = vi.fn(() => Promise.reject(new Error('name too long')))
      renderEditor({ initial: formFromCollection(withScoped), initialCatalogs: [scoped, ...library], onCopyToLibrary })
      openMenu('Scoped')
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Copy into library' }))
      expect(await screen.findByText("Couldn't copy: name too long")).toBeInTheDocument()
    })

    it('is offered only for a catalog that lives in this collection', async () => {
      renderEditor({ initial: formFromCollection(withScoped), initialCatalogs: [scoped, ...library] })
      openMenu('Late Night')
      expect(await screen.findByRole('menuitem', { name: 'Remove from folder' })).toBeInTheDocument()
      expect(screen.queryByRole('menuitem', { name: 'Copy into library' })).toBeNull()
    })
  })

  describe('the Add catalogs dropdown', () => {
    const open = () => fireEvent.click(screen.getByRole('button', { name: 'Add catalogs' }))
    const dropdown = () => screen.getByRole('dialog', { name: 'Add catalogs to this folder' })
    const folderRows = () => screen.getAllByRole('listitem').filter((row) => row.classList.contains('run-row'))

    it('ticks what the folder holds, states each catalog’s kind, and adds a tick at once, in the order ticked', () => {
      const { onSave } = renderEditor({ initial: formFromCollection(saved), initialCatalogs: library })
      open()
      expect(within(dropdown()).getByLabelText(/Late Night/)).toBeChecked()
      expect(within(dropdown()).getByText('Series')).toBeInTheDocument()
      expect(within(dropdown()).getByRole('searchbox')).not.toHaveFocus()
      fireEvent.click(within(dropdown()).getByLabelText(/Noir/))
      expect(folderRows()).toHaveLength(2)
      fireEvent.click(within(dropdown()).getByLabelText(/Giallo/))
      expect(within(dropdown()).getByLabelText(/Giallo/)).toBeChecked()
      expect(folderRows()).toHaveLength(3)
      save()
      expect(onSave).toHaveBeenCalledWith(
        expect.objectContaining({
          folders: [
            expect.objectContaining({
              catalogs: [{ catalog_id: 'c1' }, { catalog_id: 'c3' }, { catalog_id: 'c2' }],
            }),
          ],
        }),
      )
    })

    it('ticks a catalog held under any genre, and takes every genre of it out on untick', async () => {
      const narrowed = {
        ...saved,
        folders: [
          folder({
            id: 'f1',
            title: 'Horror',
            refs: [{ catalog_id: 'c2', genre: 'Giallo' }, { catalog_id: 'c1', genre: 'Horror' }, { catalog_id: 'c1', genre: 'War' }],
          }),
        ],
      }
      const { onSave } = renderEditor({ initial: formFromCollection(narrowed), initialCatalogs: library })
      open()
      expect(within(dropdown()).getByLabelText(/Giallo/)).toBeChecked()
      fireEvent.click(within(dropdown()).getByLabelText(/Late Night/))
      expect(within(dropdown()).getByLabelText(/Late Night/)).not.toBeChecked()
      fireEvent.keyDown(dropdown(), { key: 'Escape' })
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      save()
      expect(onSave).toHaveBeenCalledWith(
        expect.objectContaining({
          folders: [expect.objectContaining({ catalogs: [{ catalog_id: 'c2', genre: 'Giallo' }] })],
        }),
      )
    })

    it('says so when the library is empty or a search matches nothing', () => {
      renderEditor({ initial: formFromCollection(saved), initialCatalogs: library, options: [] })
      open()
      expect(within(dropdown()).getByText('No catalogs in your library yet.')).toBeInTheDocument()
    })

    it('offers a new catalog named for a search that matches nothing', () => {
      renderEditor({ initial: formFromCollection(saved), initialCatalogs: library })
      open()
      fireEvent.change(within(dropdown()).getByRole('searchbox'), { target: { value: 'Slasher' } })
      expect(within(dropdown()).getByText('No catalogs match this search.')).toBeInTheDocument()
      fireEvent.click(within(dropdown()).getByRole('button', { name: 'New catalog “Slasher”' }))
      expect(screen.getByRole('dialog', { name: 'New catalog' })).toBeInTheDocument()
      expect(screen.getByLabelText('Name')).toHaveValue('Slasher')
    })
  })

  describe('Unlink from library', () => {
    const openMenu = (name: string) =>
      fireEvent.pointerDown(screen.getByRole('button', { name: `More for ${name}` }), { button: 0, ctrlKey: false, pointerType: 'mouse' })

    it('moves the row to a catalog only this collection has, in its place, which Save writes', async () => {
      const twoRefs = {
        ...saved,
        folders: [folder({ id: 'f1', title: 'Horror', refs: [{ catalog_id: 'c1', genre: '' }, { catalog_id: 'c2', genre: '' }] })],
      }
      const { onSave } = renderEditor({ initial: formFromCollection(twoRefs), initialCatalogs: library })
      openMenu('Late Night')
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Unlink from library' }))
      expect(screen.getByRole('button', { name: 'Edit' })).toBeInTheDocument()
      save()
      const [payload] = onSave.mock.calls[0] as [{ folders: { catalogs: { catalog_id?: string; new?: { name: string } }[] }[] }]
      expect(payload.folders[0].catalogs).toEqual([
        { new: expect.objectContaining({ name: 'Late Night' }) },
        { catalog_id: 'c2' },
      ])
    })

    it('is not offered for a catalog that already lives in this collection', async () => {
      const scoped = catalog({ id: 's1', name: 'Scoped', collection_id: saved.id })
      const withScoped = { ...saved, folders: [folder({ id: 'f1', title: 'Horror', refs: [{ catalog_id: 's1', genre: '' }] })] }
      renderEditor({ initial: formFromCollection(withScoped), initialCatalogs: [scoped] })
      openMenu('Scoped')
      expect(await screen.findByRole('menuitem', { name: 'Copy into library' })).toBeInTheDocument()
      expect(screen.queryByRole('menuitem', { name: 'Unlink from library' })).toBeNull()
    })
  })

  it('saves a renamed folder and a catalog widened back to no genre filter', () => {
    const narrowed = {
      ...saved,
      folders: [folder({ id: 'f1', title: 'Horror', refs: [{ catalog_id: 'c1', genre: 'Horror' }] })],
    }
    const { onSave } = renderEditor({ initial: formFromCollection(narrowed), initialCatalogs: library })
    fireEvent.change(screen.getByLabelText('Title of folder 1'), { target: { value: 'Frights' } })
    fireEvent.click(screen.getByRole('button', { name: 'Genres of Late Night' }))
    const genres = screen.getByRole('dialog', { name: 'Late Night by genre' })
    expect(within(genres).getByLabelText('Horror')).toBeDisabled()
    fireEvent.click(within(genres).getByLabelText('No genre filter'))
    fireEvent.click(within(genres).getByLabelText('Horror'))
    save()
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        folders: [expect.objectContaining({ id: 'f1', title: 'Frights', catalogs: [{ catalog_id: 'c1' }] })],
      }),
    )
  })

  describe('splitting a catalog by genre', () => {
    beforeEach(() => {
      vi.mocked(apiFetch).mockImplementation((path) => {
        if (path !== '/api/catalogs/genre-options') return new Promise(() => {})
        const genres = [{ id: 27, name: 'Horror' }, { id: 35, name: 'Comedy' }]
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(genres) } as Response)
      })
    })
    afterEach(() => {
      vi.mocked(apiFetch).mockImplementation(() => new Promise(() => {}))
    })

    it('makes one row per ticked genre in a collection drawn as rows, with the dropdown open throughout', async () => {
      const { onSave } = renderEditor({ initial: formFromCollection(saved), initialCatalogs: library })
      fireEvent.click(screen.getByRole('button', { name: 'Split by genre' }))
      const genres = () => screen.getByRole('dialog', { name: 'Late Night by genre' })
      fireEvent.click(await within(genres()).findByLabelText('Horror'))
      fireEvent.click(within(genres()).getByLabelText('Comedy'))
      expect(within(genres()).getByLabelText('No genre filter')).toBeChecked()
      expect(screen.getByText('3 rows')).toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: 'Remove the Comedy row' }))
      expect(screen.getByText('2 rows')).toBeInTheDocument()
      save()
      expect(onSave).toHaveBeenCalledWith(
        expect.objectContaining({
          folders: [expect.objectContaining({ catalogs: [{ catalog_id: 'c1' }, { catalog_id: 'c1', genre: 'Horror' }] })],
        }),
      )
    })

    it('says tab for a tabbed collection, and flags a genre the recipe no longer allows', async () => {
      const split = {
        ...saved,
        view_mode: 'TABBED_GRID',
        folders: [folder({ id: 'f1', title: 'Horror', refs: [{ catalog_id: 'c1', genre: 'Horror' }, { catalog_id: 'c1', genre: 'Western' }] })],
      }
      renderEditor({ initial: formFromCollection(split), initialCatalogs: library })
      expect(screen.getByText('2 tabs')).toBeInTheDocument()
      expect(await screen.findByText(/no longer allow Western, so Nuvio shows that tab unfiltered/)).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Remove the Western tab' })).toBeInTheDocument()
    })
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
