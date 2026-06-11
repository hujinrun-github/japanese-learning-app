import { describe, expect, it } from 'vitest'
import type { ModuleStat } from '@/types/api'
import { getDailyProgress, getMasteryProgress, hasDailyGoal } from '../progress'

function stat(overrides: Partial<ModuleStat>): ModuleStat {
  return {
    due_count: 0,
    mastered_count: 0,
    total_count: 0,
    today_completed: 0,
    daily_goal: 0,
    ...overrides,
  }
}

describe('home learning map progress', () => {
  it('uses today completion for the learning map even when mastery data exists', () => {
    const moduleStat = stat({
      mastered_count: 80,
      total_count: 100,
      today_completed: 2,
      daily_goal: 20,
    })

    expect(getDailyProgress(moduleStat)).toBe(10)
    expect(getMasteryProgress(moduleStat)).toBe(80)
  })

  it('detects modules that do not have a daily target', () => {
    expect(hasDailyGoal(stat({ daily_goal: 0 }))).toBe(false)
    expect(hasDailyGoal(stat({ daily_goal: 3 }))).toBe(true)
  })
})
