import { describe, expect, it, vi } from 'vitest'
import { groupedByCatalog, newFolder, newRef, type FolderRefState } from './collectionForm'
import type { RefOption } from './refs'
import {
  catalogOrder,
  dragItemName,
  errorRoleLabels,
  genreChoices,
  holdsCatalog,
  isUnsplit,
  withCatalogAdded,
  withCatalogMoved,
  withCatalogOrder,
  withGenreAdded,
  withGenreOrder,
  withoutCatalog,
  withoutRef,
} from './folderEdits'

// The catalog form's one value import from the API barrel, which would
// otherwise load the auth session and its `window` listener.
vi.mock('@/api', () => ({ CATALOG_PROVIDER: 'tmdb' }))

const pairs = (refs: FolderRefState[]) => refs.map((ref) => [ref.catalogID, ref.genre])

describe('folder ref edits', () => {
  const western = newRef('c1', 'Western')
  const war = newRef('c1', 'War')
  const other = newRef('c2')
  const refs = [western, war, other]
  const folder = { ...newFolder(), refs }

  it('holds a catalog under any genre, and lists each catalog once', () => {
    expect(holdsCatalog(refs, 'c1')).toBe(true)
    expect(holdsCatalog(refs, 'c3')).toBe(false)
    expect(catalogOrder(refs)).toEqual(['c1', 'c2'])
  })

  it('loads a catalog’s refs side by side, where it first appears', () => {
    const loose = [western, other, war]
    expect(pairs(groupedByCatalog(loose))).toEqual([['c1', 'Western'], ['c1', 'War'], ['c2', '']])
  })

  it('moves whole catalogs, each keeping its genres in order', () => {
    expect(pairs(withCatalogOrder(refs, ['c2', 'c1']))).toEqual([['c2', ''], ['c1', 'Western'], ['c1', 'War']])
    expect(pairs(withCatalogOrder(refs, ['c2', 'gone']))).toEqual([['c2', ''], ['c1', 'Western'], ['c1', 'War']])
  })

  it('reorders one catalog’s genres in its place', () => {
    expect(pairs(withGenreOrder(refs, 'c1', [war.key, 'gone']))).toEqual([['c1', 'War'], ['c1', 'Western'], ['c2', '']])
  })

  it('adds a genre after the catalog’s last, never twice and never to a catalog it doesn’t hold', () => {
    expect(pairs(withGenreAdded(refs, 'c1', ''))).toEqual([['c1', 'Western'], ['c1', 'War'], ['c1', ''], ['c2', '']])
    expect(withGenreAdded(refs, 'c1', 'War')).toBe(refs)
    expect(withGenreAdded(refs, 'c3', 'War')).toBe(refs)
  })

  it('removes a genre, but never a catalog’s only one', () => {
    expect(withoutRef(refs, war.key)).toEqual([western, other])
    expect(withoutRef(refs, other.key)).toBe(refs)
    expect(withoutRef(refs, 'gone')).toBe(refs)
  })

  it('adds a catalog unsplit at the end, unless it is held under any genre', () => {
    expect(pairs(withCatalogAdded(refs, 'c3'))).toEqual([['c1', 'Western'], ['c1', 'War'], ['c2', ''], ['c3', '']])
    expect(withCatalogAdded(refs, 'c1')).toBe(refs)
  })

  it('removes and moves a whole catalog', () => {
    expect(withoutCatalog(refs, 'c1')).toEqual([other])
    expect(pairs(withCatalogMoved(refs, 'c1', 'draft:1'))).toEqual([['draft:1', 'Western'], ['draft:1', 'War'], ['c2', '']])
  })

  it('calls a catalog unsplit only when it is one ref with no genre filter', () => {
    expect(isUnsplit([other])).toBe(true)
    expect(isUnsplit([western])).toBe(false)
    expect(isUnsplit([western, war])).toBe(false)
  })

  it('names a dragged catalog by id and a dragged genre by ref key', () => {
    const optionByID = new Map([['c1', { name: 'Hidden Gems' } as RefOption]])
    expect(dragItemName([folder], optionByID, 'c1')).toBe('Hidden Gems')
    expect(dragItemName([folder], optionByID, war.key)).toBe('Hidden Gems, War')
    expect(dragItemName([folder], optionByID, other.key)).toBe('this catalog')
  })
})

describe('genreChoices', () => {
  const horror = newRef('c1', 'Horror')
  const western = newRef('c1', 'Western')

  it('offers no genre filter, the recipe’s genres, then a stored genre they lack, marked stale', () => {
    expect(genreChoices(['Horror', 'Comedy'], [horror, western])).toEqual([
      { genre: '', refKey: undefined, stale: false },
      { genre: 'Horror', refKey: horror.key, stale: false },
      { genre: 'Comedy', refKey: undefined, stale: false },
      { genre: 'Western', refKey: western.key, stale: true },
    ])
  })

  it('marks nothing stale before the options land', () => {
    expect(genreChoices(undefined, [western])).toEqual([
      { genre: '', refKey: undefined, stale: false },
      { genre: 'Western', refKey: western.key, stale: false },
    ])
  })
})

describe('errorRoleLabels', () => {
  it('names the title, then each folder’s problems by its place', () => {
    const first = newFolder()
    const second = newFolder()
    const errors = { title: 'Give this collection a title.', folders: { [second.key]: { title: 'x', catalogIDs: 'y' } } }
    expect(errorRoleLabels(errors, [first, second])).toEqual(['Title', 'folder 2’s title', 'folder 2’s catalogs'])
    expect(errorRoleLabels({ folders: { [first.key]: { catalogIDs: 'y' } } }, [first])).toEqual(['folder 1’s catalogs'])
    expect(errorRoleLabels({ title: 't', folderCount: 'n', folders: {} }, [first])).toEqual(['Title', 'Folders'])
  })
})
