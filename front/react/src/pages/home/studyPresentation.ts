import type { TodayStudyPlan } from '@/api/studyplan'

type Mode = 'legacy' | 'loading' | 'error' | 'unknown' | 'before_start' | 'ended' | 'rest_day' | 'study_day'
interface Presentation {
  mode: Mode
  useLegacyGoals: boolean
  goal: string | null
  heading: string | null
  description: string | null
  mapMessage: string | null
}

export function homeStudyPresentation(plan: TodayStudyPlan | null, loading: boolean, error: string): Presentation {
  let mode: Mode = 'unknown'
  if (loading) mode = 'loading'
  else if (error) mode = 'error'
  else if (plan) {
    if (plan.reason === 'before_start' || plan.reason === 'ended') mode = plan.reason
    else if (plan.rest_day || plan.reason === 'rest_day') mode = 'rest_day'
    else if (plan.reason === 'no_plan' || plan.reason === 'legacy') mode = 'legacy'
    else if (plan.reason === 'study_day') mode = 'study_day'
  }

  if (mode === 'legacy') return { mode, useLegacyGoals: true, goal: null, heading: null, description: null, mapMessage: null }
  if (mode === 'study_day') return { mode, useLegacyGoals: false, goal: `${plan!.total_minutes} 分钟`, heading: '按时段完成今天的学习', description: plan!.title, mapMessage: '今日任务以上方时段安排为准' }

  const messages = {
    loading: { goal: '读取中', heading: '正在读取学习安排', description: '读取完成后查看今天的时段和任务。' },
    error: { goal: '暂不可用', heading: '学习安排暂时无法读取', description: '请重新读取今日任务，或查看学习计划。' },
    unknown: { goal: '待确认', heading: '查看学习安排', description: '查看学习计划，确认今天的安排。' },
    before_start: { goal: '尚未开始', heading: '计划尚未开始', description: '今天没有计划任务，到开始日期后按安排学习。' },
    ended: { goal: '已结束', heading: '本期计划已结束', description: '今天没有计划任务，可调整日期继续学习。' },
    rest_day: { goal: '休息日', heading: '今天安心休息', description: '今天没有计划任务，下个学习日继续。' },
  }
  const paused = mode === 'before_start' || mode === 'ended' || mode === 'rest_day'
  return { mode, useLegacyGoals: false, ...messages[mode], mapMessage: paused ? '今天不安排任务，可自由浏览' : '学习安排以今日任务为准' }
}
