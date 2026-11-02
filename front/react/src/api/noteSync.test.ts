import { describe, expect, it } from 'vitest'
import { normalizeNoteSyncTargets } from './noteSync'
import type { NoteSyncTarget } from '../types/api'

describe('normalizeNoteSyncTargets', () => {
  const target = { id: 1, name: 'Notion' } as NoteSyncTarget

  it.each([
    ['null response', null, []],
    ['undefined response', undefined, []],
    ['empty response', [], []],
    ['populated response', [target], [target]],
  ])('%s', (_name, input, expected) => {
    expect(normalizeNoteSyncTargets(input)).toEqual(expected)
  })
})
