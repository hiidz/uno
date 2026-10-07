import { describe, expect, it } from 'vitest'
import { catalog, collection, folder } from '@/test/fixtures'
import { catalogDeleteConsequences, collectionDeleteConsequences } from './deleteConsequences'

const live = { id: 'p1', changed_since_publish: false }

const using = (id: string, home: number | null = null) =>
  collection({
    id: `k-${id}`,
    home_position: home,
    folders: [folder({ refs: [{ catalog_id: id, genre: '' }] })],
  })

describe('catalogDeleteConsequences', () => {
  it('says nothing for a catalog nothing else holds', () => {
    expect(catalogDeleteConsequences(catalog({ id: 'a' }), [using('b')])).toEqual({
      removedFrom: null,
      addersKeep: false,
      scopedCatalogs: 0,
    })
  })

  it('names Community and the adders only while published', () => {
    expect(catalogDeleteConsequences(catalog({ publication: live }), [])).toMatchObject({
      removedFrom: 'Also removes it from: Community',
      addersKeep: true,
    })
    expect(catalogDeleteConsequences(catalog({ publication: null }), [])).toMatchObject({
      removedFrom: null,
      addersKeep: false,
    })
  })

  it('counts the collections whose folders use it', () => {
    const rows = [using('a'), using('a'), using('b')]
    expect(catalogDeleteConsequences(catalog({ id: 'a' }), rows).removedFrom).toBe(
      'Also removes it from: 2 collections',
    )
  })

  it('names Nuvio for its own Home place or a using collection', () => {
    expect(catalogDeleteConsequences(catalog({ home_position: 0 }), []).removedFrom).toBe(
      'Also removes it from: Nuvio (next push)',
    )
    expect(catalogDeleteConsequences(catalog({ id: 'a' }), [using('a', 3)]).removedFrom).toBe(
      'Also removes it from: 1 collection · Nuvio (next push)',
    )
  })

  it('lists everything in order', () => {
    expect(
      catalogDeleteConsequences(catalog({ id: 'a', publication: live, home_position: 1 }), [
        using('a'),
      ]).removedFrom,
    ).toBe('Also removes it from: Community · 1 collection · Nuvio (next push)')
  })
})

describe('collectionDeleteConsequences', () => {
  it('says nothing for a collection that is nowhere else', () => {
    expect(collectionDeleteConsequences(collection({ id: 'k' }))).toEqual({
      removedFrom: null,
      addersKeep: false,
      scopedCatalogs: 0,
    })
  })

  it('names Community and Nuvio, and counts its scoped catalogs', () => {
    const row = collection({
      id: 'k',
      publication: live,
      home_position: 0,
      catalogs: [catalog({ id: 'x', collection_id: 'k' }), catalog({ id: 'y', collection_id: null })],
    })
    expect(collectionDeleteConsequences(row)).toEqual({
      removedFrom: 'Also removes it from: Community · Nuvio (next push)',
      addersKeep: true,
      scopedCatalogs: 1,
    })
  })
})
