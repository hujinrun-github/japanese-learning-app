import { describe, expect, it } from 'vitest'
import { catalogParams, getCatalogFilters } from './lessonCatalog'

describe('lesson list URL', () => {
  it.each([
    ['', { level: '', q: '', page: 1 }, ''],
    ['level=N2&q=+お互いさま+&page=3', { level: 'N2', q: 'お互いさま', page: 3 }, 'level=N2&q=%E3%81%8A%E4%BA%92%E3%81%84%E3%81%95%E3%81%BE&page=3'],
    ['level=N9&page=-1', { level: '', q: '', page: 1 }, ''],
    ['level=N1&page=1.5', { level: 'N1', q: '', page: 1 }, 'level=N1'],
  ] as const)('normalizes %s', (raw, expected, canonical) => {
    const filters = getCatalogFilters(new URLSearchParams(raw))
    expect(filters).toEqual(expected)
    expect(catalogParams(filters).toString()).toBe(canonical)
  })
})
