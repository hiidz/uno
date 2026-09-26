import { describe, expect, it, vi } from 'vitest'
import type { Catalog, Collection } from '@/api'
import { formFromCatalog, toPayload as toCatalogPayload } from '@/features/catalogs/catalogForm'
import type { CatalogFormState } from '@/features/catalogs/catalogForm'
import { catalog, collection, folder } from '@/test/fixtures'
import {
  changesContent,
  emptyCollectionForm,
  formFromCollection,
  isSameCollection,
  newFolder,
  newRef,
  toCollectionPayload,
  validateCollectionForm,
  withCatalogEdit,
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
        move_to_library: false,
      },
    ])
  })

  it('drops the edit again once the catalog is edited back', () => {
    const renamed = withCatalogEdit(baseline, saved, { ...formFromCatalog(saved), name: 'Renamed' })
    const reverted = withCatalogEdit(renamed, saved, formFromCatalog(saved))
    expect(reverted.catalogEdits).toEqual({})
    expect(isSameCollection(baseline, reverted)).toBe(true)
  })

  it('marks Move to library', () => {
    const moved = { ...formFromCatalog(saved), collectionID: null }
    const [edit] = toCollectionPayload(withCatalogEdit(baseline, saved, moved)).catalog_edits
    expect(edit.move_to_library).toBe(true)
  })
})

// The collection editor's Undo on a staged Move to library: the catalog as the
// editor holds it after the nested save (name and params from that save's
// payload), put back into the collection, then re-read through the form.
describe('undoing a staged Move to library', () => {
  // Keys out of order and a rolling date window, the two ways a stored
  // recipe differs from what the form writes back.
  const saved = catalog({
    name: 'Scoped',
    params: '{"sort_by": "popularity.desc", "released_within_days": 30, "with_genres": "28"}',
    collection_id: 'col1',
  })
  const baseline = formWith([newRef('c1')])

  function stageAndUndo(nested: CatalogFormState) {
    const staged = withCatalogEdit(baseline, saved, nested)
    const payload = toCatalogPayload(nested)
    const held: Catalog = { ...saved, name: payload.name, params: payload.params, collection_id: null }
    return withCatalogEdit(staged, saved, formFromCatalog({ ...held, collection_id: saved.collection_id }))
  }

  it('leaves the form clean when the move was the only change', () => {
    expect(toCatalogPayload(formFromCatalog(saved)).params).not.toBe(saved.params)
    const undone = stageAndUndo({ ...formFromCatalog(saved), collectionID: null })
    expect(undone.catalogEdits).toEqual({})
    expect(isSameCollection(baseline, undone)).toBe(true)
  })

  it('keeps a rename made alongside the move', () => {
    const undone = stageAndUndo({ ...formFromCatalog(saved), name: 'Renamed', collectionID: null })
    expect(undone.catalogEdits.c1).toMatchObject({ name: 'Renamed', moveToLibrary: false })
  })
})

describe('changesContent', () => {
  const baseline = formWith([newRef('c1')])

  it('ignores a Public toggle on its own', () => {
    expect(changesContent(baseline, { ...baseline, isPublic: true })).toBe(false)
  })

  it('counts a change to anything else', () => {
    expect(changesContent(baseline, { ...baseline, isPublic: true, title: 'Renamed' })).toBe(true)
  })

  it('counts a staged Move to library with nothing else changed', () => {
    const edit = { type: 'movie', provider: 'tmdb', name: 'Scoped', params: '{}', moveToLibrary: true } as const
    expect(changesContent(baseline, { ...baseline, catalogEdits: { c1: edit } })).toBe(true)
  })
})

describe('view mode', () => {
  function stored(viewMode: string): Collection {
    return collection({ view_mode: viewMode })
  }

  it('loads an empty or unknown view mode as Tabbed Grids, what Nuvio shows for one', () => {
    for (const mode of ['', 'SIDEWAYS']) {
      expect(formFromCollection(stored(mode)).viewMode).toBe('TABBED_GRID')
    }
  })

  it('starts a new folder as Poster and loads an empty tile shape as Poster, what Nuvio shows for one', () => {
    expect(newFolder().tileShape).toBe('POSTER')
    const saved = collection({ view_mode: 'ROWS', folders: [folder({ tile_shape: '' })] })
    expect(formFromCollection(saved).folders[0].tileShape).toBe('POSTER')
  })

  it('saves each named view mode back as itself', () => {
    for (const mode of ['FOLLOW_LAYOUT', 'TABBED_GRID', 'ROWS']) {
      expect(toCollectionPayload(formFromCollection(stored(mode))).view_mode).toBe(mode)
    }
  })
})
