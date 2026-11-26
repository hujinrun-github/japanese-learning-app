import { describe, expect, it } from 'vitest'
import { ACCOUNT_NAV_ITEMS, DESKTOP_NAV_GROUPS, DESKTOP_PRIMARY_ITEMS, MOBILE_MORE_ITEMS, MOBILE_PRIMARY_ITEMS, activeDesktopGroup } from './navigation'

describe('responsive navigation', () => {
  it('keeps every learner module reachable on desktop and mobile', () => {
    const requiredRoutes = [
      '/',
      '/search',
      '/words/review',
      '/grammar',
      '/lesson',
      '/listening',
      '/speaking',
      '/writing',
      '/notes',
      '/translation',
      '/settings',
      '/errata',
      '/mistakes',
      '/favorites',
      '/study-plan',
    ]

    const desktopRoutes = new Set([
      ...DESKTOP_PRIMARY_ITEMS, ...DESKTOP_NAV_GROUPS.flatMap(group => group.items), ...ACCOUNT_NAV_ITEMS,
    ].map(item => item.to))
    const mobileRoutes = new Set(
      [...MOBILE_PRIMARY_ITEMS, ...MOBILE_MORE_ITEMS].map((item) => item.to),
    )

    for (const route of requiredRoutes) {
      expect(desktopRoutes.has(route)).toBe(true)
      expect(mobileRoutes.has(route)).toBe(true)
    }
  })

  it('keeps home and search direct and gives each group distinct destinations', () => {
    expect(DESKTOP_PRIMARY_ITEMS.map(item => item.to)).toEqual(['/', '/search'])
    expect(DESKTOP_NAV_GROUPS.map(group => [group.id, group.items.map(item => item.to)])).toEqual([
      ['learn', ['/words/review', '/grammar', '/lesson']],
      ['practice', ['/listening', '/speaking', '/writing', '/translation']],
      ['organize', ['/notes', '/favorites', '/mistakes', '/study-plan']],
    ])
  })

  it.each([
    ['/grammar/41', 'learn'],
    ['/lesson/8/shadowing', 'learn'],
    ['/listening/8', 'practice'],
    ['/translation/practice/grammar/41', 'practice'],
    ['/notes/new', 'organize'],
    ['/favorites', 'organize'],
    ['/study-plan', 'organize'],
    ['/search/words/15', null],
    ['/listening-extra', null],
    ['/', null],
  ])('highlights the right group at %s', (pathname, expected) => {
    expect(activeDesktopGroup(pathname)).toBe(expected)
  })

  it('keeps search directly accessible in the fixed mobile tab bar', () => {
    expect(MOBILE_PRIMARY_ITEMS.map((item) => item.to)).toContain('/search')
    expect(MOBILE_PRIMARY_ITEMS).toHaveLength(5)
  })
})
