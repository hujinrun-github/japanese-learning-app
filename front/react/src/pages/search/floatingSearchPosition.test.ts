import { describe, expect, it } from 'vitest'
import { clampFloatingSearchPosition } from './floatingSearchPosition'

describe('clampFloatingSearchPosition', () => {
  it.each([
    { name: 'keeps an in-bounds position', left: 70, top: 120, width: 300, height: 220, expected: { left: 70, top: 120 } },
    { name: 'keeps the handle visible at the top left', left: -20, top: -40, width: 300, height: 220, expected: { left: 8, top: 8 } },
    { name: 'keeps the panel inside the bottom right', left: 400, top: 700, width: 300, height: 220, expected: { left: 82, top: 552 } },
    { name: 'prioritizes the top left if the panel is larger than the viewport', left: 40, top: 50, width: 430, height: 800, expected: { left: 8, top: 8 } },
  ])('$name', ({ left, top, width, height, expected }) => {
    expect(clampFloatingSearchPosition({ left, top, width, height }, { width: 390, height: 780 })).toEqual(expected)
  })
})
