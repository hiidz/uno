import { describe, expect, it } from 'vitest'
import { catalog, collection, folder } from '@/test/fixtures'
import {
  errorText,
  isPublished,
  ownSharing,
  publishGroups,
  rowStickers,
  sharingNote,
  stickerWords,
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

describe('rowStickers', () => {
  const labels = (row: Parameters<typeof rowStickers>[0]) => rowStickers(row).map((s) => `${s.label}:${s.tone}`)

  it('says an own row is published, and changed since', () => {
    expect(labels({ publication: null, subscription: null })).toEqual([])
    expect(labels({ publication: live, subscription: null })).toEqual(['Published:published'])
    expect(labels({ publication: { ...live, changed_since_publish: true }, subscription: null })).toEqual([
      'Published:published',
      'Changed:quiet',
    ])
    expect(labels({ publication: { ...live, status: 'unpublished' }, subscription: null })).toEqual([])
  })

  it('says a row came from Community, with its update or its unpublishing', () => {
    expect(labels({ publication: null, subscription })).toEqual(['From Community:from'])
    expect(labels({ publication: null, subscription: { ...subscription, update_available: true } })).toEqual([
      'From Community:from',
      'Update:update',
    ])
    expect(labels({ publication: null, subscription: { ...subscription, unpublished: true } })).toEqual([
      'From Community:from',
      'Unpublished:quiet',
    ])
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
    expect(stickerWords([{ label: 'Published', tone: 'published' }, { label: 'Changed', tone: 'quiet' }])).toBe(', published, changed')
    expect(stickerWords([])).toBe('')
  })
})
