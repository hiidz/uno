import { describe, expect, it } from 'vitest'
import type { PendingChange } from '@/api'
import { catalog, collection, folder } from '@/test/fixtures'
import {
  errorText,
  isPublished,
  ownSharing,
  publishGroups,
  railStickers,
  rowStickers,
  sharingNote,
  stickerWords,
  viewStickers,
  waitingIDs,
  type SharingSticker,
} from './sharingState'

const live = { id: 'p', status: 'live' as const, changed_since_publish: false }
const subscription = { publication_id: 'p', update_available: false, unpublished: false }

describe('ownSharing', () => {
  it('reads each publication state', () => {
    expect(ownSharing(null)).toBe('private')
    expect(ownSharing(live)).toBe('live')
    expect(ownSharing({ ...live, changed_since_publish: true })).toBe('changed')
    expect(ownSharing({ ...live, status: 'unpublished', changed_since_publish: true })).toBe('unpublished')
  })

  it('counts only a live publication as published', () => {
    expect(isPublished(catalog({ publication: live }))).toBe(true)
    expect(isPublished(catalog({ publication: { ...live, status: 'unpublished' } }))).toBe(false)
    expect(isPublished(catalog())).toBe(false)
  })
})

type StickerRow = Parameters<typeof rowStickers>[0]
const words = (stickers: SharingSticker[]) => stickers.map((s) => `${s.label}:${s.tone}`)

const updating: StickerRow = { publication: null, subscription: { ...subscription, update_available: true } }
const unpublishedByPublisher: StickerRow = { publication: null, subscription: { ...subscription, unpublished: true } }
const changed: StickerRow = { publication: { ...live, changed_since_publish: true }, subscription: null }

describe('railStickers', () => {
  const rail = (row: StickerRow) => words(railStickers(row))

  it('carries one Community sticker, changing with the row’s state', () => {
    expect(rail({ publication: null, subscription: null })).toEqual([])
    expect(rail({ publication: live, subscription: null })).toEqual(['Published:community'])
    expect(rail(changed)).toEqual(['Publish changes:community'])
    expect(rail({ publication: { ...live, status: 'unpublished' }, subscription: null })).toEqual([])
    expect(rail({ publication: null, subscription })).toEqual(['From Community:community'])
    expect(rail(updating)).toEqual(['Update available:update'])
  })

  it('leaves the publisher’s unpublishing to the editor and the Home pane', () => {
    expect(rail(unpublishedByPublisher)).toEqual(['From Community:community'])
  })
})

describe('rowStickers', () => {
  const all = (row: StickerRow, waiting = false) => words(rowStickers(row, waiting))

  it('adds Push to Nuvio to the Community sticker while a push would change Nuvio', () => {
    expect(all({ publication: null, subscription: null })).toEqual([])
    expect(all({ publication: null, subscription: null }, true)).toEqual(['Push to Nuvio:push'])
    expect(all(changed, true)).toEqual(['Publish changes:community', 'Push to Nuvio:push'])
    expect(all(updating, true)).toEqual(['Update available:update', 'Push to Nuvio:push'])
  })

  it('says a publisher unpublished a row added from Community', () => {
    expect(all(unpublishedByPublisher)).toEqual(['From Community:community', 'Unpublished:quiet'])
    expect(all(unpublishedByPublisher, true)).toEqual([
      'From Community:community',
      'Unpublished:quiet',
      'Push to Nuvio:push',
    ])
  })

  it('never says "update" except for an incoming one', () => {
    const labels = [changed, updating, unpublishedByPublisher, { publication: live, subscription: null }]
      .flatMap((row) => rowStickers(row, true))
      .filter((s) => /update/i.test(s.label))
    expect(words(labels)).toEqual(['Update available:update'])
  })
})

describe('viewStickers', () => {
  it('says From Community where Update available would be, since Update… says it', () => {
    expect(words(viewStickers(updating, false))).toEqual(['From Community:community'])
    expect(words(viewStickers(updating, true))).toEqual(['From Community:community', 'Push to Nuvio:push'])
    expect(words(viewStickers({ publication: null, subscription }, false))).toEqual(['From Community:community'])
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

describe('sharingNote', () => {
  it('asks for a save before publishing what is saved', () => {
    expect(sharingNote('private', true)).toBe('Save first: only what’s saved is published.')
    expect(sharingNote('changed', true)).toBe('Save first: only what’s saved is published.')
    expect(sharingNote('live', true)).toBeNull()
    expect(sharingNote('private', false)).toBeNull()
  })
})

describe('errorText and stickerWords', () => {
  it('reads a failed call’s message', () => {
    expect(errorText(new Error('boom'))).toBe('boom')
    expect(errorText(null)).toBeNull()
  })

  it('reads stickers as words for a screen reader', () => {
    expect(stickerWords(rowStickers(changed, true))).toBe(', publish changes, push to nuvio')
    expect(stickerWords([])).toBe('')
  })
})
