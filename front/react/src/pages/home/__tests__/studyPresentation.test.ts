import { describe, expect, it } from 'vitest'
import type { TodayStudyPlan } from '@/api/studyplan'
import { homeStudyPresentation } from '../studyPresentation'

const studyDay: TodayStudyPlan = { has_plan: true, date: '2026-11-19', timezone: 'Asia/Shanghai', title: 'N2 备考', reason: 'study_day', rest_day: false, total_minutes: 100, slots: [] }

describe('首页学习安排呈现', () => {
  it.each([
    { name: '加载中且已有旧数据', plan: studyDay, loading: true, error: '', goal: '读取中', heading: '正在读取学习安排' },
    { name: '读取失败且已有旧数据', plan: studyDay, loading: false, error: '网络失败', goal: '暂不可用', heading: '学习安排暂时无法读取' },
    { name: '未知状态', plan: null, loading: false, error: '', goal: '待确认', heading: '查看学习安排' },
    { name: '计划尚未开始', plan: { ...studyDay, reason: 'before_start' as const, total_minutes: 0 }, loading: false, error: '', goal: '尚未开始', heading: '计划尚未开始' },
    { name: '计划已结束', plan: { ...studyDay, reason: 'ended' as const, total_minutes: 0 }, loading: false, error: '', goal: '已结束', heading: '本期计划已结束' },
    { name: '休息日', plan: { ...studyDay, reason: 'rest_day' as const, rest_day: true, total_minutes: 0 }, loading: false, error: '', goal: '休息日', heading: '今天安心休息' },
  ])('$name 不展示旧目标或要求学习0分钟', ({ plan, loading, error, goal, heading }) => {
    const view = homeStudyPresentation(plan, loading, error)
    expect(view.useLegacyGoals).toBe(false)
    expect(view.goal).toBe(goal)
    expect(view.heading).toBe(heading)
    expect(view.heading).not.toContain('按时段完成')
    expect(view.goal).not.toBe('0 分钟')
  })

  it.each(['no_plan', 'legacy'] as const)('仅明确%s并且不是休息日时使用旧目标', reason => {
    expect(homeStudyPresentation({ ...studyDay, reason }, false, '').useLegacyGoals).toBe(true)
    expect(homeStudyPresentation({ ...studyDay, reason, rest_day: true }, false, '').useLegacyGoals).toBe(false)
  })

  it('只有当天为学习日时呈现分钟目标及学习号召', () => {
    const view = homeStudyPresentation(studyDay, false, '')
    expect(view.goal).toBe('100 分钟')
    expect(view.heading).toBe('按时段完成今天的学习')
    expect(view.description).toBe('N2 备考')
  })
})
