import { describe, expect, it, vi } from 'vitest'
import { catalog } from '@/test/fixtures'
import { newFolder, newRef } from './collectionForm'
import { errorRoleLabels, nestedCatalogForm, withGenreRef, withRefs } from './folderEdits'

// The catalog form's one value import from the API barrel, which would
// otherwise load the auth session and its `window` listener.
vi.mock('@/api', () => ({ CATALOG_PROVIDER: 'tmdb' }))

describe('folder ref edits', () => {
  const western = newRef('c1', 'Western')
  const other = newRef('c2')
  const folder = { ...newFolder(), refs: [western, other] }

  it('adds another genre of a ref directly below it', () => {
    const next = withGenreRef(folder, folder.key, western.key, 'War')
    expect(next.refs.map((ref) => [ref.catalogID, ref.genre])).toEqual([
      ['c1', 'Western'],
      ['c1', 'War'],
      ['c2', ''],
    ])
  })

  it('leaves another folder, an unknown ref and a genre already there alone', () => {
    expect(withGenreRef(folder, 'another folder', western.key, 'War')).toBe(folder)
    expect(withGenreRef(folder, folder.key, 'no such ref', 'War')).toBe(folder)
    expect(withGenreRef(folder, folder.key, other.key, '')).toBe(folder)
  })

  it('passes only the named folder’s refs through an update', () => {
    const update = (refs: ReturnType<typeof newRef>[]) => refs.slice(1)
    expect(withRefs(folder, folder.key, update).refs).toEqual([other])
    expect(withRefs(folder, 'another folder', update)).toBe(folder)
  })
})

describe('errorRoleLabels', () => {
  it('names the title, then each folder’s problems by its place', () => {
    const first = newFolder()
    const second = newFolder()
    const errors = { title: 'Give this collection a title.', folders: { [second.key]: { title: 'x', catalogIDs: 'y' } } }
    expect(errorRoleLabels(errors, [first, second])).toEqual(['Title', 'folder 2’s title', 'folder 2’s catalogs'])
    expect(errorRoleLabels({ folders: { [first.key]: { catalogIDs: 'y' } } }, [first])).toEqual(['folder 1’s catalogs'])
  })
})

describe('nestedCatalogForm', () => {
  it('is the open catalog’s form, or nothing', () => {
    expect(nestedCatalogForm(catalog({ name: 'Scoped' }))).toMatchObject({ name: 'Scoped' })
    expect(nestedCatalogForm(undefined)).toBeUndefined()
  })
})
