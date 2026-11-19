import { describe, expect, it } from 'vitest'
import { REVIEW_ITEMS } from '../reviewQueue'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'
import ja from '@/i18n/locales/ja'

// Regression: the entry labeled 错题本 used to open /writing (写作练习).
describe('home review queue', () => {
  for (const tc of [
    { key: 'word', to: '/words/review?level=all', countFrom: 'module' },
    { key: 'grammar', to: '/grammar', countFrom: 'module' },
    { key: 'writing', to: '/mistakes', countFrom: 'mistakes' },
    { key: 'speaking', to: '/speaking', countFrom: 'module' },
  ]) {
    it(`keeps the ${tc.key} entry aligned with its destination`, () => {
      const item = REVIEW_ITEMS.find(entry => entry.key === tc.key)
      expect(item?.to).toBe(tc.to)
      expect(item?.countFrom).toBe(tc.countFrom)
    })
  }

  it('labels the mistake book entry consistently in all three languages', () => {
    expect(zh.home.reviewQueue.writing).toBe('错题本')
    expect(en.home.reviewQueue.writing).toBe('Mistake Book')
    expect(ja.home.reviewQueue.writing).toBe('間違いノート')
  })
})
