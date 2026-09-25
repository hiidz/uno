import { expect, it } from 'vitest'
import { ordinal } from './ordinal'

it('gives each number its English suffix, with the teens as th', () => {
  const cases: [number, string][] = [
    [1, '1st'],
    [2, '2nd'],
    [3, '3rd'],
    [4, '4th'],
    [11, '11th'],
    [12, '12th'],
    [13, '13th'],
    [21, '21st'],
    [22, '22nd'],
    [23, '23rd'],
    [101, '101st'],
    [111, '111th'],
    [112, '112th'],
  ]
  for (const [n, expected] of cases) expect(ordinal(n)).toBe(expected)
})
