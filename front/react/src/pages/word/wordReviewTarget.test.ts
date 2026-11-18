import { describe, expect, it } from 'vitest'
import { wordReviewTargetID } from './wordReviewTarget'

describe('word review target ID', () => {
  it.each([
    { value: '7', expected: 7 },
    { value: '482', expected: 482 },
    ...[null, '', '0', '-1', '1.5', '7abc', ' 7', '9007199254740993'].map(value => ({ value, expected: null })),
  ])('validates $value', ({ value, expected }) => {
    expect(wordReviewTargetID(value)).toBe(expected)
  })
})
