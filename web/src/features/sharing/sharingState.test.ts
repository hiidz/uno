import { describe, expect, it } from 'vitest'
import type { PendingChange } from '@/api'
import { catalog, collection, folder } from '@/test/fixtures'
import {
  kindStickers,
  ownSharing,
  publishGroups,
  railStickers,
  rowStickers,
  sharingStep,
  stickerWords,
  viewStickers,
  waitingIDs,
  type SharingSticker,
} from './sharingState'

const live = { id: 'p', changed_since_publish: false }
const subscription = { publication_id: 'p', update_available: false }

describe('ownSharing', () => {
  it('reads each publication state', () => {
    expect(ownSharing(null)).toBe('private')
    expect(ownSharing(live)).toBe('live')
    expect(ownSharing({ ...live, changed_since_publish: true })).toBe('changed')
  })
})

type StickerRow = Parameters<typeof rowStickers>[0]
const words = (stickers: SharingSticker[]) => stickers.map((s) => `${s.label}:${s.tone.hue}${s.tone.fill ? ', filled' : ''}`)

const plain: StickerRow = { publication: null, subscription: null }
const updating: StickerRow = { ...plain, subscription: { ...subscription, update_available: true } }
const changed: StickerRow = { ...plain, publication: { ...live, changed_since_publish: true } }

describe('railStickers', () => {
  const rail = (row: StickerRow) => words(railStickers(row))

  it('carries one Community sticker, changing with the row’s state', () => {
    expect(rail(plain)).toEqual([])
    expect(rail({ ...plain, publication: live })).toEqual(['Published:community'])
    expect(rail(changed)).toEqual(['To publish:community, filled'])
    expect(rail({ ...plain, subscription })).toEqual(['From Community:community'])
    expect(rail(updating)).toEqual(['Update available:community, filled'])
  })
})

describe('rowStickers', () => {
  const all = (row: StickerRow, waiting = false) => words(rowStickers(row, waiting))

  it('adds To push to the Community sticker while a push would change Nuvio', () => {
    expect(all(plain)).toEqual([])
    expect(all(plain, true)).toEqual(['To push:nuvio, filled'])
    expect(all(changed, true)).toEqual(['To publish:community, filled', 'To push:nuvio, filled'])
    expect(all(updating, true)).toEqual(['Update available:community, filled', 'To push:nuvio, filled'])
  })

  it('never says "update" except for an incoming one', () => {
    const labels = [changed, updating, { ...plain, publication: live }]
      .flatMap((row) => rowStickers(row, true))
      .filter((s) => /update/i.test(s.label))
    expect(words(labels)).toEqual(['Update available:community, filled'])
  })
})

describe('viewStickers', () => {
  it('says From Community where Update available would be, since Update… says it', () => {
    expect(words(viewStickers(updating, false))).toEqual(['From Community:community'])
    expect(words(viewStickers(updating, true))).toEqual(['From Community:community', 'To push:nuvio, filled'])
    expect(words(viewStickers({ ...plain, subscription }, false))).toEqual(['From Community:community'])
  })
})

describe('waitingIDs', () => {
  const pending = (kind: PendingChange['kind'], id: string, change: PendingChange['change']): PendingChange => ({
    kind,
    id,
    name: id,
    change,
  })

  it('holds the added and the changed rows of both kinds, and not the removed', () => {
    const ids = waitingIDs([
      pending('catalog', 'c1', 'added'),
      pending('catalog', 'c2', 'changed'),
      pending('catalog', 'c3', 'removed'),
      pending('collection', 'k1', 'changed'),
      pending('collection', 'k2', 'removed'),
    ])
    expect([...ids].sort()).toEqual(['c1', 'c2', 'k1'])
  })

  it('is empty when nothing waits', () => {
    expect(waitingIDs([]).size).toBe(0)
  })
})

describe('publishGroups', () => {
  const scoped = catalog({ id: 's1', name: 'Scoped', collection_id: 'col1' })
  const listed = catalog({ id: 'l1', name: 'Listed', subscription })
  const unused = catalog({ id: 'u1', name: 'Unused' })
  const tree = collection({
    folders: [
      folder({ refs: [{ catalog_id: 's1', genre: '' }, { catalog_id: 'l1', genre: 'Horror' }] }),
      folder({ id: 'f2', refs: [{ catalog_id: 'l1', genre: '' }, { catalog_id: 'gone', genre: '' }] }),
    ],
    catalogs: [scoped, listed, unused],
  })

  it('lists each used catalog once, own and library apart', () => {
    expect(publishGroups(tree)).toEqual({ own: [scoped], library: [listed] })
  })

  it('reads a tree with nothing in it', () => {
    expect(publishGroups({ id: 'x', folders: null, catalogs: null })).toEqual({ own: [], library: [] })
    expect(publishGroups({ id: 'x', folders: [folder({ refs: null })], catalogs: [] })).toEqual({ own: [], library: [] })
  })
})

describe('sharingStep', () => {
  it('names the next step for each state', () => {
    expect(sharingStep('private', false).label).toBe('Publish…')
    expect(sharingStep('changed', false).label).toBe('Publish update…')
    expect(sharingStep('live', false).label).toBe('Unpublish…')
  })

  it('waits for a save while the form has unsaved changes', () => {
    expect(sharingStep('private', true).waiting).toBe('Save first.')
    expect(sharingStep('live', true).waiting).toBe('Save first.')
    expect(sharingStep('changed', false).waiting).toBeNull()
  })
})

describe('stickerWords', () => {
  it('reads stickers as words for a screen reader', () => {
    expect(stickerWords(rowStickers(changed, true))).toBe(', to publish, to push')
    expect(stickerWords([])).toBe('')
  })
})

describe('kindStickers', () => {
  it('names a catalog’s kind in the catalogs’ tangerine, and nothing for an unknown one', () => {
    expect(words(kindStickers({ type: 'series' }))).toEqual(['Series:catalog'])
    expect(kindStickers(undefined)).toEqual([])
  })
})
