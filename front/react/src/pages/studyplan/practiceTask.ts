import type { StudyPracticeProgress, TodayStudyPlan, TodayStudyTask } from '@/api/studyplan'

export function practiceTaskProgress(task: TodayStudyTask): StudyPracticeProgress | undefined {
  if (task.progress) return task.progress
  if (task.word_review) return { ...task.word_review, kind: 'word_review' }
}

export function isAutomaticPracticeCheckIn(task: TodayStudyTask) {
  return task.completion_reason === 'word_review_goal' || task.completion_reason === 'grammar_goal'
}

export function practiceTaskLink(task: TodayStudyTask, slotID: string) {
  const progress = practiceTaskProgress(task)
  const url = new URL(task.path, 'https://local.invalid')
  if (!progress && url.pathname !== '/grammar') return task.path
  const params = url.searchParams
  if (progress?.kind === 'word_review') {
    params.set('level', progress.level)
    params.set('queue', 'review')
  }
  params.set('plan_slot', slotID)
  params.set('plan_task', task.id)
  return `${url.pathname}${url.search}${url.hash}`
}

export function findPracticeTask(plan: TodayStudyPlan | null, kind: StudyPracticeProgress['kind'], level: string | null, params: URLSearchParams) {
  const tasks = plan?.slots.flatMap(slot => slot.tasks.flatMap(task => {
    const progress = practiceTaskProgress(task)
    return progress?.kind === kind ? [{ slotID: slot.id, task, progress }] : []
  })) ?? []
  if (params.has('plan_slot') || params.has('plan_task')) {
    return tasks.find(item => item.slotID === params.get('plan_slot') && item.task.id === params.get('plan_task'))
  }
  if (kind === 'grammar') {
    const matching = tasks.filter(item => item.progress.level === level || item.progress.level === 'all')
    return matching.length === 1 ? matching[0] : tasks.length === 1 ? tasks[0] : undefined
  }
  return tasks.find(item => item.progress.level === level)
    ?? tasks.find(item => item.progress.level === 'all')
    ?? (tasks.length === 1 ? tasks[0] : undefined)
}

export function withPracticeTask(path: string, params: URLSearchParams) {
  const slot = params.get('plan_slot')
  const task = params.get('plan_task')
  if (!slot || !task) return path
  const url = new URL(path, 'https://local.invalid')
  url.searchParams.set('plan_slot', slot)
  url.searchParams.set('plan_task', task)
  return `${url.pathname}${url.search}${url.hash}`
}
