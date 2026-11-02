import { describe, expect, it } from 'vitest'
import type { DailyPlan, DailyPlanItem } from '@/types/api'
import { isPlanComplete, pickNextPlanItem } from '../dailyPlan'

function item(key: DailyPlanItem['key'], status: DailyPlanItem['status']): DailyPlanItem {
  return {
    key,
    path: `/${key}`,
    goal: 10,
    completed: status === 'done' ? 10 : status === 'in_progress' ? 4 : 0,
    remaining: status === 'done' ? 0 : status === 'in_progress' ? 6 : 10,
    due_count: 0,
    status,
    percent: status === 'done' ? 100 : status === 'in_progress' ? 40 : 0,
  }
}

function plan(items: DailyPlanItem[]): DailyPlan {
  return {
    date: '2026-10-29',
    timezone: 'Asia/Shanghai',
    streak_days: 0,
    total_goal: items.reduce((sum, i) => sum + i.goal, 0),
    total_completed: items.reduce((sum, i) => sum + i.completed, 0),
    remaining: items.reduce((sum, i) => sum + i.remaining, 0),
    percent: 0,
    completed: items.every(i => i.status === 'done'),
    items,
  }
}

describe('pickNextPlanItem', () => {
  it('returns null for a missing or empty plan', () => {
    expect(pickNextPlanItem(null)).toBeNull()
    expect(pickNextPlanItem(plan([]))).toBeNull()
  })

  it('returns null when every module is done', () => {
    expect(pickNextPlanItem(plan([item('word', 'done'), item('grammar', 'done')]))).toBeNull()
  })

  it('prefers a started module over an untouched one', () => {
    const next = pickNextPlanItem(plan([item('word', 'todo'), item('writing', 'in_progress')]))
    expect(next?.key).toBe('writing')
  })

  it('falls back to canonical module order among untouched modules', () => {
    const next = pickNextPlanItem(plan([item('writing', 'todo'), item('grammar', 'todo')]))
    expect(next?.key).toBe('grammar')
  })

  it('places translation after the existing practice modules', () => {
    const next = pickNextPlanItem(plan([item('translation', 'todo'), item('writing', 'todo')]))
    expect(next?.key).toBe('writing')
  })

  it('keeps canonical order among started modules', () => {
    const next = pickNextPlanItem(plan([item('writing', 'in_progress'), item('word', 'in_progress')]))
    expect(next?.key).toBe('word')
  })

  it('skips completed modules when choosing', () => {
    const next = pickNextPlanItem(plan([item('word', 'done'), item('speaking', 'todo')]))
    expect(next?.key).toBe('speaking')
  })
})

describe('isPlanComplete', () => {
  it('is false without a plan or items', () => {
    expect(isPlanComplete(null)).toBe(false)
    expect(isPlanComplete(plan([]))).toBe(false)
  })

  it('is true only when every module is done', () => {
    expect(isPlanComplete(plan([item('word', 'done'), item('grammar', 'done')]))).toBe(true)
    expect(isPlanComplete(plan([item('word', 'done'), item('grammar', 'in_progress')]))).toBe(false)
  })
})
