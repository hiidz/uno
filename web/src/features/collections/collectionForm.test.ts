import { describe, expect, it, vi } from 'vitest'
import type { Catalog } from '@/api'
import { formFromCatalog, toPayload as toCatalogPayload } from '@/features/catalogs/catalogForm'
import {
  emptyCollectionForm,
  isSameCollection,
  newFolder,
  newRef,
  reorderRefs,
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
    const draft: Catalog = {
      id: 'draft:d1',
      type: 'movie',
      name: 'Staged',
      provider: 'tmdb',
      params: '{}',
      owner_id: '',
      is_public: false,
      collection_id: 'col1',
      created_at: '',
      updated_at: '',
    }
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
  const saved: Catalog = {
    id: 'c1',
    type: 'movie',
    name: 'Scoped',
    provider: 'tmdb',
    params: '{"with_genres": "28", "sort_by": "popularity.desc"}',
    owner_id: '',
    is_public: false,
    collection_id: 'col1',
    created_at: '',
    updated_at: '',
  }
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

describe('reorderRefs', () => {
  it('orders refs by key, so two refs to one catalog keep their own genres', () => {
    const a = newRef('c1', 'Western')
    const b = newRef('c1', 'War')
    expect(reorderRefs([a, b], [b.key, a.key])).toEqual([b, a])
  })
})
