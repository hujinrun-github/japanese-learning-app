import { describe, expect, it } from 'vitest'
import { mistakePageWithinTotal } from './mistakePagination'

describe('mistake page after status change', () => {
  it.each([
    { page: 2, total: 21, expected: 2 },
    { page: 2, total: 20, expected: 1 },
    { page: 3, total: 20, expected: 1 },
    { page: 2, total: 0, expected: 1 },
    { page: 1, total: 40, expected: 1 },
  ])('$page with $total mistakes -> $expected', ({ page, total, expected }) => {
    expect(mistakePageWithinTotal(page, total, 20)).toBe(expected)
  })
})
