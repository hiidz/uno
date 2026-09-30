import { describe, expect, it } from 'vitest'
import { catalog, collection, folder } from '@/test/fixtures'
import {
  blockedReason,
  errorText,
  fromWords,
  isShared,
  ownSharing,
  publishGroups,
  rowStickers,
  sharingNote,
  stickerWords,
  subscribedCatalogsIn,
} from './sharingState'

const live = { id: 'p', status: 'live' as const, changed_since_publish: false }
const subscription = { publication_id: 'p', update_available: false, withdrawn: false }

describe('ownSharing', () => {
  it('reads each publication state', () => {
    expect(ownSharing(null)).toBe('private')
    expect(ownSharing(live)).toBe('live')
    expect(ownSharing({ ...live, changed_since_publish: true })).toBe('changed')
    expect(ownSharing({ ...live, status: 'withdrawn', changed_since_publish: true })).toBe('withdrawn')
  })

  it('counts only a live publication as shared', () => {
    expect(isShared(catalog({ publication: live }))).toBe(true)
    expect(isShared(catalog({ publication: { ...live, status: 'withdrawn' } }))).toBe(false)
    expect(isShared(catalog())).toBe(false)
  })
})

describe('rowStickers', () => {
  const labels = (row: Parameters<typeof rowStickers>[0]) => rowStickers(row).map((s) => `${s.label}:${s.tone}`)

  it('says an own row is shared, and changed since', () => {
    expect(labels({ publication: null, subscription: null })).toEqual([])
    expect(labels({ publication: live, subscription: null })).toEqual(['Shared:shared'])
    expect(labels({ publication: { ...live, changed_since_publish: true }, subscription: null })).toEqual([
      'Shared:shared',
      'Changed:quiet',
    ])
    expect(labels({ publication: { ...live, status: 'withdrawn' }, subscription: null })).toEqual([])
  })

  it('says a copy came from Community, with its update or its withdrawal', () => {
    expect(labels({ publication: null, subscription })).toEqual(['From Community:from'])
    expect(labels({ publication: null, subscription: { ...subscription, update_available: true } })).toEqual([
      'From Community:from',
      'Update:update',
    ])
    expect(labels({ publication: null, subscription: { ...subscription, withdrawn: true } })).toEqual([
      'From Community:from',
      'No longer shared:quiet',
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

  it('finds the library catalogs taken from Community', () => {
    expect(subscribedCatalogsIn(tree)).toEqual([listed])
    expect(subscribedCatalogsIn(collection())).toEqual([])
  })

  it('reads a tree with nothing in it', () => {
    expect(publishGroups({ id: 'x', folders: null, catalogs: null })).toEqual({ own: [], library: [] })
    expect(publishGroups({ id: 'x', folders: [folder({ refs: null })], catalogs: [] })).toEqual({ own: [], library: [] })
  })
})

describe('blockedReason', () => {
  it('is null when nothing blocks the share', () => {
    expect(blockedReason([])).toBeNull()
  })

  it('names the taken catalogs and what to do about them', () => {
    expect(blockedReason([catalog({ name: 'Giallo' })])).toBe(
      'Uses 1 catalog taken from Community: “Giallo”. To share this collection, detach it, or duplicate it and use the copy instead. Duplicating the collection won’t help: its duplicate uses the same catalogs.',
    )
    expect(blockedReason([catalog({ name: 'A' }), catalog({ name: 'B' })])).toContain(
      'Uses 2 catalogs taken from Community: “A” and “B”. To share this collection, detach them, or duplicate them and use the copies instead.',
    )
  })
})

describe('sharingNote', () => {
  it('says why a row can’t be shared before anything else', () => {
    expect(sharingNote('private', true, 'Blocked.')).toBe('Blocked.')
  })

  it('asks for a save before sharing what is saved', () => {
    expect(sharingNote('private', true, null)).toBe('Save first: sharing shares what’s saved.')
    expect(sharingNote('changed', true, null)).toBe('Save first: sharing shares what’s saved.')
    expect(sharingNote('live', true, null)).toBeNull()
    expect(sharingNote('private', false, null)).toBeNull()
  })
})

describe('errorText and stickerWords', () => {
  it('reads a failed call’s message', () => {
    expect(errorText(new Error('boom'))).toBe('boom')
    expect(errorText(null)).toBeNull()
  })

  it('reads stickers as words for a screen reader', () => {
    expect(stickerWords([{ label: 'Shared', tone: 'shared' }, { label: 'Changed', tone: 'quiet' }])).toBe(', shared, changed')
    expect(stickerWords([])).toBe('')
  })
})

describe('fromWords', () => {
  it('says whether a copy follows its owner, has an update waiting, or gets no more', () => {
    expect(fromWords(subscription)).toBe('Taken from Community. It follows its owner’s updates until you save a change to it.')
    expect(fromWords({ ...subscription, update_available: true })).toBe('Taken from Community. Its owner has published an update.')
    expect(fromWords({ ...subscription, withdrawn: true })).toBe(
      'Taken from Community. Its owner stopped sharing it, so it gets no more updates.',
    )
  })
})
