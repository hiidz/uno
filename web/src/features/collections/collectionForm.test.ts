import { describe, expect, it, vi } from 'vitest'
import type { Collection, ViewMode } from '@/api'
import { formFromCatalog, toPayload as toCatalogPayload } from '@/features/catalogs/catalogForm'
import { catalog, collection, folder } from '@/test/fixtures'
import {
  MAX_FOLDERS,
  MAX_REFS_PER_FOLDER,
  appearanceSummary,
  countErrors,
  emptyCollectionForm,
  formFromCollection,
  isSameCollection,
  newFolder,
  newRef,
  ownErrors,
  toCollectionPayload,
  validateCollectionForm,
  withCatalogEdit,
  VIEW_MODES,
  type CollectionFormState,
} from './collectionForm'

// The catalog form's one value import from the API barrel, which would
// otherwise load the auth session and its `window` listener.
vi.mock('@/api', () => ({ CATALOG_PROVIDER: 'tmdb' }))

function formWith(refs: ReturnType<typeof newRef>[]): CollectionFormState {
  return { ...emptyCollectionForm(), title: 'C', folders: [{ ...newFolder(), title: 'F', refs }] }
}

describe('validateCollectionForm', () => {
  const accessible = new Set(['c1'])

  it('allows one catalog twice under different genres', () => {
    const form = formWith([newRef('c1', 'Western'), newRef('c1', 'War'), newRef('c1')])
    expect(validateCollectionForm(form, accessible).folders).toEqual({})
  })

  it('flags one catalog twice under the same genre', () => {
    const form = formWith([newRef('c1', 'War'), newRef('c1', 'War')])
    const errors = Object.values(validateCollectionForm(form, accessible).folders)
    expect(errors[0]?.catalogIDs).toBe('This folder lists the same catalog with the same genre twice.')
  })

  it('names catalogs gone from the library before anything else', () => {
    const many = Object.values(validateCollectionForm(formWith([newRef('gone'), newRef('gone')]), accessible).folders)
    expect(many[0]?.catalogIDs).toBe('2 catalogs here are no longer available. Remove them to save.')
    const one = Object.values(validateCollectionForm(formWith([newRef('gone')]), accessible).folders)
    expect(one[0]?.catalogIDs).toBe('One catalog here is no longer available. Remove it to save.')
  })

  it('holds a folder to twenty refs and a collection to ten folders', () => {
    const genres = (n: number) => Array.from({ length: n }, (_, i) => newRef('c1', `G${i}`))
    expect(validateCollectionForm(formWith(genres(MAX_REFS_PER_FOLDER)), accessible).folders).toEqual({})
    const over = Object.values(validateCollectionForm(formWith(genres(MAX_REFS_PER_FOLDER + 2)), accessible).folders)
    expect(over[0]?.catalogIDs).toMatch(/at most 20 catalogs.*Remove 2 to save\.$/)

    const folders = (n: number) => Array.from({ length: n }, () => ({ ...newFolder(), title: 'F' }))
    const atCap = validateCollectionForm({ ...emptyCollectionForm(), title: 'C', folders: folders(MAX_FOLDERS) }, accessible)
    expect(atCap.folderCount).toBeUndefined()
    const errors = validateCollectionForm({ ...emptyCollectionForm(), title: '', folders: folders(MAX_FOLDERS + 1) }, accessible)
    expect(errors.folderCount).toBe('A collection holds at most 10 folders. Remove 1 to save.')
    expect(ownErrors(errors)).toEqual(['Give this collection a title.', errors.folderCount])
    expect(countErrors(errors)).toBe(2)
  })

  it('holds titles to 200 characters, counted as characters', () => {
    const wide = '日'.repeat(200)
    const at = validateCollectionForm({ ...emptyCollectionForm(), title: wide, folders: [{ ...newFolder(), title: wide }] }, accessible)
    expect(countErrors(at)).toBe(0)
    const over = validateCollectionForm(
      { ...emptyCollectionForm(), title: `${wide}日`, folders: [{ ...newFolder(), title: `${wide}日` }] },
      accessible,
    )
    expect(over.title).toBe('Keep the title to 200 characters or fewer.')
    expect(Object.values(over.folders)[0]?.title).toBe('Keep the title to 200 characters or fewer.')
  })

  it('takes media addresses that are http or https, and says which one is not', () => {
    const media = (update: Partial<ReturnType<typeof newFolder>>) => {
      const form = { ...emptyCollectionForm(), title: 'C', folders: [{ ...newFolder(), title: 'F', ...update }] }
      return Object.values(validateCollectionForm(form, accessible).folders)[0]?.media
    }
    expect(media({ coverImageURL: 'https://x.test/a.png', heroVideoURL: ' http://x.test/v.mp4 ' })).toBeUndefined()
    expect(media({ coverImageURL: 'javascript:alert(1)' })).toBe(
      'Cover image must be a web address starting with http:// or https://.',
    )
    expect(media({ titleLogoURL: 'not a url' })).toMatch(/^Title logo must be a web address/)
    expect(media({ focusGIFURL: `https://x.test/${'a'.repeat(2048)}` })).toBe('Focus GIF address is too long.')

    const backdrop = validateCollectionForm(
      { ...emptyCollectionForm(), title: 'C', backdropImageURL: 'data:image/png;base64,AA' },
      accessible,
    )
    expect(backdrop.backdrop).toMatch(/^Background image must be a web address/)
    expect(countErrors(backdrop)).toBe(1)
  })
})

describe('toCollectionPayload', () => {
  it('sends each ref in order, with a genre only when one is set', () => {
    const form = formWith([newRef('c1', 'Western'), newRef('c1')])
    expect(toCollectionPayload(form).folders[0].catalogs).toEqual([
      { catalog_id: 'c1', genre: 'Western' },
      { catalog_id: 'c1' },
    ])
  })

  it('keys every ref to one draft with its draft id', () => {
    const draft = catalog({ id: 'draft:d1', name: 'Staged', collection_id: 'col1' })
    const form = formWith([newRef(draft.id, 'Action'), newRef(draft.id, 'Comedy')])
    const spec = { key: 'draft:d1', type: 'movie', name: 'Staged', provider: 'tmdb', params: '{}' }
    expect(toCollectionPayload(form, new Map([[draft.id, draft]])).folders[0].catalogs).toEqual([
      { new: spec, genre: 'Action' },
      { new: spec, genre: 'Comedy' },
    ])
  })
})

describe('withCatalogEdit', () => {
  // Deliberately not byte-for-byte what the catalog form writes (it never
  // writes whitespace), the way a row saved by another client can be.
  const saved = catalog({
    name: 'Scoped',
    params: '{"with_genres": "28", "sort_by": "popularity.desc"}',
    collection_id: 'col1',
  })
  const baseline = formWith([newRef('c1')])

  it('leaves no edit for a nested save that changes nothing, however the stored params are spelled', () => {
    const untouched = formFromCatalog(saved)
    expect(toCatalogPayload(untouched).params).not.toBe(saved.params)
    const state = withCatalogEdit(baseline, saved, untouched)
    expect(state.catalogEdits).toEqual({})
    expect(isSameCollection(baseline, state)).toBe(true)
  })

  it('stages a real edit, which dirties the form and reaches the payload', () => {
    const renamed = { ...formFromCatalog(saved), name: 'Renamed' }
    const state = withCatalogEdit(baseline, saved, renamed)
    expect(isSameCollection(baseline, state)).toBe(false)
    expect(toCollectionPayload(state).catalog_edits).toEqual([
      {
        id: 'c1',
        type: 'movie',
        provider: 'tmdb',
        name: 'Renamed',
        params: toCatalogPayload(renamed).params,
      },
    ])
  })

  it('drops the edit again once the catalog is edited back', () => {
    const renamed = withCatalogEdit(baseline, saved, { ...formFromCatalog(saved), name: 'Renamed' })
    const reverted = withCatalogEdit(renamed, saved, formFromCatalog(saved))
    expect(reverted.catalogEdits).toEqual({})
    expect(isSameCollection(baseline, reverted)).toBe(true)
  })
})

describe('view mode', () => {
  function stored(viewMode: ViewMode): Collection {
    return collection({ view_mode: viewMode })
  }

  it('starts a new collection as Tabbed Grids and a new folder as Poster', () => {
    expect(emptyCollectionForm().viewMode).toBe('TABBED_GRID')
    expect(newFolder().tileShape).toBe('POSTER')
  })

  it('loads a stored tile shape as itself', () => {
    const saved = collection({ view_mode: 'ROWS', folders: [folder({ tile_shape: 'SQUARE' })] })
    expect(formFromCollection(saved).folders[0].tileShape).toBe('SQUARE')
  })

  it('saves each view mode back as itself', () => {
    for (const mode of VIEW_MODES) {
      expect(toCollectionPayload(formFromCollection(stored(mode))).view_mode).toBe(mode)
    }
  })
})

describe('appearanceSummary', () => {
  const base = { viewMode: 'TABBED_GRID' as const, showAllTab: false, backdropImageURL: '', focusGlowEnabled: false }

  it('names the view mode, then what else is on', () => {
    expect(appearanceSummary(base)).toBe('Tabbed Grids')
    expect(appearanceSummary({ ...base, showAllTab: true, focusGlowEnabled: true })).toBe('Tabbed Grids · All tab · glow on')
    expect(appearanceSummary({ ...base, backdropImageURL: ' https://x ' })).toBe('Tabbed Grids · background image')
  })

  it('leaves the All tab out where there are no tabs', () => {
    expect(appearanceSummary({ ...base, viewMode: 'ROWS', showAllTab: true })).toBe('Rows')
  })
})
