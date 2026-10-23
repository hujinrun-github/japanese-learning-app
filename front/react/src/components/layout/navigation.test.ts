import { describe, expect, it } from 'vitest'
import { DESKTOP_NAV_ITEMS, MOBILE_MORE_ITEMS, MOBILE_PRIMARY_ITEMS } from './navigation'

describe('responsive navigation', () => {
  it('keeps every learner module reachable on desktop and mobile', () => {
    const requiredRoutes = [
      '/',
      '/words/review',
      '/grammar',
      '/lesson',
      '/speaking',
      '/writing',
      '/notes',
      '/translation',
      '/settings',
    ]

    const desktopRoutes = new Set(DESKTOP_NAV_ITEMS.map((item) => item.to))
    const mobileRoutes = new Set(
      [...MOBILE_PRIMARY_ITEMS, ...MOBILE_MORE_ITEMS].map((item) => item.to),
    )

    for (const route of requiredRoutes) {
      expect(desktopRoutes.has(route)).toBe(true)
      expect(mobileRoutes.has(route)).toBe(true)
    }
  })

  it('limits the fixed mobile tab bar to four learning destinations', () => {
    expect(MOBILE_PRIMARY_ITEMS).toHaveLength(4)
  })
})
