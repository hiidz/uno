import { describe, expect, it } from 'vitest'
import { parseIdList, parseParams, serializeIdList } from './params'

describe('parseParams', () => {
  it('reads a stored recipe, and anything unreadable as no filters', () => {
    expect(parseParams('{"sort_by":"popularity.desc"}')).toEqual({ sort_by: 'popularity.desc' })
    expect(parseParams('')).toEqual({})
    expect(parseParams('not json')).toEqual({})
    expect(parseParams('3')).toEqual({})
  })
})

describe('parseIdList / serializeIdList', () => {
  it('reads commas as all and pipes as any', () => {
    expect(parseIdList('420,2')).toEqual({ ids: [420, 2], join: 'and' })
    expect(parseIdList('420|2')).toEqual({ ids: [420, 2], join: 'or' })
  })

  it('reads empty and missing as no ids', () => {
    expect(parseIdList(undefined)).toEqual({ ids: [], join: 'and' })
    expect(parseIdList('')).toEqual({ ids: [], join: 'and' })
  })

  it('drops parts that are not positive whole ids', () => {
    expect(parseIdList(' 420 ,,abc,-3,0, 2 ').ids).toEqual([420, 2])
    expect(parseIdList('x|7|').ids).toEqual([7])
    expect(parseIdList('1.5,3').ids).toEqual([3])
  })

  it('round-trips both joins', () => {
    for (const raw of ['420,2,7', '420|2|7', '99']) {
      const { ids, join } = parseIdList(raw)
      expect(serializeIdList(ids, join)).toBe(raw)
    }
  })

  it('writes the join it is given', () => {
    expect(serializeIdList([1, 2], 'and')).toBe('1,2')
    expect(serializeIdList([1, 2], 'or')).toBe('1|2')
    expect(serializeIdList([], 'or')).toBe('')
  })
})
