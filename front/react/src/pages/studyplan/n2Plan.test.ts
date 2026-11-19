import { describe, expect, it } from 'vitest'
import { createN2Plan, tasksForWeekday, slotEndTime, changeStudyWeekdays, resolveStudyTaskPath, OFFICIAL_N2_MATERIALS } from './n2Plan'

describe('N2 工作日计划', () => {
  it.each([1, 2, 3, 4, 5])('周 %i 的任务恰好安排 100 分钟，分三次完成', day => {
    const plan = createN2Plan(new Date('2026-11-19T04:00:00Z'))
    const slots = plan.time_slots ?? []
    expect(slots.map(slot => [slot.time, slot.duration_minutes])).toEqual([
      ['12:50', 20], ['18:40', 20], ['22:00', 60],
    ])
    expect(slots.reduce((sum, slot) => sum + tasksForWeekday(slot.tasks, day).reduce((n, task) => n + task.minutes, 0), 0)).toBe(100)
  })

  it.each([0, 6])('周 %i 没有任何任务', day => {
    const plan = createN2Plan(new Date('2026-11-19T04:00:00Z'))
    expect(plan.weekdays).not.toContain(day)
    expect(plan.time_slots?.flatMap(slot => tasksForWeekday(slot.tasks, day))).toEqual([])
  })

  it('周一用于补漏，周五只测验复盘，周二至周四学新语法', () => {
    const night = createN2Plan(new Date('2026-11-19T04:00:00Z')).time_slots![2]
    expect(tasksForWeekday(night.tasks, 1).map(task => task.title)).toEqual(['补上周漏洞', '阅读精读', '错题复盘'])
    expect(tasksForWeekday(night.tasks, 3).map(task => task.title)).toEqual(['学习新语法', '阅读精读', '错题复盘'])
    expect(tasksForWeekday(night.tasks, 5).map(task => task.minutes)).toEqual([45, 15])
  })

  it('按上海日期开始，备考到明年年中，包含 N2 练习入口', () => {
    const plan = createN2Plan(new Date('2026-11-19T17:00:00Z'))
    expect(plan.start_date).toBe('2026-11-20')
    expect(plan.end_date).toBe('2027-06-30')
    expect(plan.timezone).toBe('Asia/Shanghai')
    expect(plan.time_slots?.[0].tasks.every(task => task.path === '/words/review?level=N2')).toBe(true)
    expect(plan.time_slots?.[1].tasks[0].path).toBe('/listening?level=N2')
  })

  it.each([
    ['listening', OFFICIAL_N2_MATERIALS, '/listening?level=N2'],
    ['weekly-test', OFFICIAL_N2_MATERIALS, OFFICIAL_N2_MATERIALS],
    ['listening', '/listening/12', '/listening/12'],
    ['listening', '/lesson?level=N2', '/lesson?level=N2'],
  ])('仅为旧官方听力任务兼容入口 %s %s', (id, path, expected) => {
    const task = { id, path }
    expect(resolveStudyTaskPath(task)).toBe(expected)
    expect(task.path).toBe(path)
  })

  it('暂时关闭周一再保存，保留全部任务；重新开启后仍为100分钟', () => {
    const original = createN2Plan(new Date('2026-11-19T04:00:00Z'))
    const disabled = changeStudyWeekdays(original, [2, 3, 4, 5])
    expect(disabled.time_slots).toEqual(original.time_slots)
    expect(disabled.time_slots?.flatMap(slot => tasksForWeekday(slot.tasks, 1, disabled.weekdays))).toEqual([])
    const saved = JSON.parse(JSON.stringify(disabled))
    const restored = changeStudyWeekdays(saved, [1, 2, 3, 4, 5])
    expect(restored.time_slots?.flatMap(slot => tasksForWeekday(slot.tasks, 1, restored.weekdays)).reduce((sum, task) => sum + task.minutes, 0)).toBe(100)
    expect(restored.time_slots?.[2].tasks.some(task => task.id === 'catch-up')).toBe(true)
  })

  it('空任务星期继承计划星期，且尊重非工作日自定义计划', () => {
    const task = { id: 'review', title: '复习', description: '', path: '/mistakes', minutes: 10, weekdays: [] }
    expect(tasksForWeekday([task], 6, [6])).toEqual([task])
    expect(tasksForWeekday([task], 1, [6])).toEqual([])
  })

  it.each([
    ['12:50', 20, '13:10'], ['18:40', 20, '19:00'], ['22:00', 60, '23:00'],
  ])('结束时间 %s + %i = %s', (time, minutes, expected) => {
    expect(slotEndTime(time, minutes)).toBe(expected)
  })
})
