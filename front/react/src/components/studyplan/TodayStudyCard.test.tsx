import { renderToStaticMarkup } from 'react-dom/server'
import { StaticRouter } from 'react-router-dom/server'
import { describe, expect, it } from 'vitest'
import type { TodayStudyPlan } from '@/api/studyplan'
import { TodayStudyCard } from './TodayStudyCard'

const today: TodayStudyPlan = {
  has_plan: true, title: 'N2 工作日备考', date: '2026-11-19', timezone: 'Asia/Shanghai', rest_day: false,
  reason: 'study_day', total_minutes: 20,
  slots: [{ id: 'lunch', title: '午间词汇', time: '12:50', duration_minutes: 20, tasks: [
    { id: 'words', title: '词汇复习', description: '先复习到期词', path: '/words/review?level=N2', minutes: 20, completed: true },
  ] }],
}

function render(plan: TodayStudyPlan, pendingTask: string | null = null) {
  return renderToStaticMarkup(<StaticRouter location="/"><TodayStudyCard plan={plan} loading={false} error="" pendingTask={pendingTask} onToggle={() => {}} /></StaticRouter>)
}

describe('今日时段任务', () => {
  it('显示当地日期、时段结束时间、完成数和 N2 入口', () => {
    const html = render(today)
    for (const expected of ['2026-11-19', '12:50', '13:10', '词汇复习', 'checked=""', '1 / 1', '/words/review?level=N2']) {
      expect(html).toContain(expected)
    }
  })

  it('休息日不显示任务和打卡控件，即使残留任务列表', () => {
    const html = render({ ...today, rest_day: true, reason: 'rest_day' })
    expect(html).toContain('今天休息')
    expect(html).not.toContain('type="checkbox"')
    expect(html).not.toContain('词汇复习')
  })

  it.each([
    ['before_start', '计划尚未开始'], ['ended', '本期计划已结束'],
  ] as const)('休息标记不覆盖 %s 的具体原因', (reason, expected) => {
    const html = render({ ...today, slots: [], rest_day: true, reason })
    expect(html).toContain(expected)
    expect(html).not.toContain('type="checkbox"')
  })

  it('链接到该语法任务，显示服务端数量和自动打卡状态', () => {
    const slot = today.slots[0]
    const task = { ...slot.tasks[0], id: 'grammar', title: '语法2项', description: '',
      path: '/grammar?level=N2', completed: true, completion_reason: 'grammar_goal',
      progress: { kind: 'grammar' as const, target: 2, completed: 2, level: 'N2' as const, due_remaining: 0 } }
    const html = render({ ...today, slots: [{ ...slot, tasks: [task] }] })
    expect(html).toContain('N2 · 今日 2/2 项 · 已自动打卡')
    expect(html).toContain('/grammar?level=N2&amp;plan_slot=lunch&amp;plan_task=grammar')
    expect(html).toContain('disabled=""')
  })

  it('没有保存计划时不显示服务器 Local 时区', () => {
    const html = render({ ...today, has_plan: false, slots: [], timezone: 'Local', reason: 'no_plan' })
    expect(html).not.toContain('Local')
    expect(html).toContain('创建工作日 N2 计划')
  })

  it('网络失败明确提示，不能假装今天没有安排', () => {
    const html = renderToStaticMarkup(<StaticRouter location="/"><TodayStudyCard plan={null} loading={false} error="读取失败" pendingTask={null} onToggle={() => {}} /></StaticRouter>)
    expect(html).toContain('role="alert"')
    expect(html).toContain('读取失败')
    expect(html).not.toContain('今天休息')
  })

  it('提交完成状态时禁用勾选，避免并发覆盖', () => {
    expect(render(today, 'lunch/words')).toContain('disabled=""')
  })

  it.each([
    { completed: 19, checked: false, reason: undefined, status: '还差 1 个，达标自动打卡' },
    { completed: 20, checked: true, reason: 'word_review_goal', status: '已自动打卡' },
    { completed: 19, checked: true, reason: 'manual', status: '已手动确认' },
  ])('计划目标显示 $completed/20 和 $status', ({ completed, checked, reason, status }) => {
    const slot = today.slots[0]
    const task = { ...slot.tasks[0], title: '复习20个单词', description: '复习20个单词，完成后在今日任务中打卡。',
      path: '/words/review', completed: checked, completion_reason: reason,
      word_review: { target: 20, completed, level: 'all' as const, due_remaining: 0 } }
    const html = render({ ...today, slots: [{ ...slot, tasks: [task] }] })
    expect(html).toContain(`今日 ${completed}/20 个`)
    expect(html).toContain(status)
    expect(html).toContain('level=all&amp;queue=review&amp;plan_slot=lunch&amp;plan_task=words')
    expect(html).not.toContain('完成后在今日任务中打卡')
    expect(html.includes('checked=""')).toBe(checked)
    expect(html.includes('disabled=""')).toBe(reason === 'word_review_goal')
  })
})
