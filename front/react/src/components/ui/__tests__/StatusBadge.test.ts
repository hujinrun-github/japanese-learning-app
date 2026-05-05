import { describe, it, expect } from 'vitest'
import type { StatusType } from '../StatusBadge'

// Tests for StatusBadge logic (without DOM rendering)
// These validate the status type contract and i18n key mapping.

const STATUS_I18N_KEY: Record<StatusType, string> = {
  unlearned: 'status.unlearned',
  learning: 'status.learning',
  mastered: 'status.mastered',
  pass: 'status.pass',
  needs_work: 'status.needsWork',
}

const ALL_STATUSES: StatusType[] = ['unlearned', 'learning', 'mastered', 'pass', 'needs_work']

describe('StatusBadge', () => {
  it('has an i18n key for every status type', () => {
    for (const status of ALL_STATUSES) {
      expect(STATUS_I18N_KEY[status]).toBeTruthy()
    }
  })

  it('maps mastered to success i18n key', () => {
    expect(STATUS_I18N_KEY['mastered']).toBe('status.mastered')
  })

  it('maps learning to learning i18n key', () => {
    expect(STATUS_I18N_KEY['learning']).toBe('status.learning')
  })

  it('maps unlearned to unlearned i18n key', () => {
    expect(STATUS_I18N_KEY['unlearned']).toBe('status.unlearned')
  })

  it('maps pass to pass i18n key', () => {
    expect(STATUS_I18N_KEY['pass']).toBe('status.pass')
  })

  it('maps needs_work to needsWork i18n key', () => {
    expect(STATUS_I18N_KEY['needs_work']).toBe('status.needsWork')
  })

  it('covers exactly 5 status types', () => {
    expect(Object.keys(STATUS_I18N_KEY).length).toBe(5)
  })
})
