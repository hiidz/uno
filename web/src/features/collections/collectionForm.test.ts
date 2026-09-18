import { describe, expect, it } from 'vitest'
import type { Catalog } from '@/api'
import {
  emptyCollectionForm,
  newFolder,
  newRef,
  reorderRefs,
  toCollectionPayload,
  validateCollectionForm,
  type CollectionFormState,
} from './collectionForm'

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

describe('reorderRefs', () => {
  it('orders refs by key, so two refs to one catalog keep their own genres', () => {
    const a = newRef('c1', 'Western')
    const b = newRef('c1', 'War')
    expect(reorderRefs([a, b], [b.key, a.key])).toEqual([b, a])
  })
})
