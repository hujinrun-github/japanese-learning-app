import { useTodayStudyPlan } from '@/components/studyplan/useTodayStudyPlan'
import type { DailyPracticeState } from '@/components/ui/DailyPracticeProgress'
import { useDailyPracticeProgress } from '@/hooks/useDailyPracticeProgress'
import { findPracticeTask } from '@/pages/studyplan/practiceTask'
import { useTranslation } from 'react-i18next'

export function useGrammarPracticeProgress(level: string | null, params: URLSearchParams) {
  const { t } = useTranslation()
  const scheduled = useTodayStudyPlan()
  const hasTask = params.has('plan_slot') || params.has('plan_task')
  const showHabit = scheduled.plan?.has_plan === false && !hasTask
  const habit = useDailyPracticeProgress('grammar', showHabit)
  const goal = findPracticeTask(scheduled.plan, 'grammar', level, params)?.progress
    ?? (hasTask ? undefined : scheduled.plan?.legacy_grammar)

  let progress: DailyPracticeState
  if (scheduled.loading) progress = { status: 'loading' }
  else if (scheduled.error) progress = { status: 'error' }
  else if (goal) progress = {
    status: 'ready', source: 'plan', completed: goal.completed, goal: goal.target,
    scope: goal.level === 'all' ? t('word.queue.allLevels') : goal.level,
  }
  else if (showHabit) progress = habit.progress.status === 'ready' ? { ...habit.progress, source: 'habit' } : habit.progress
  else progress = { status: hasTask ? 'taskUnavailable' : 'notScheduled' }

  function refreshProgress() {
    if (showHabit) habit.refreshProgress()
    scheduled.reload()
  }
  return { progress, refreshProgress }
}
