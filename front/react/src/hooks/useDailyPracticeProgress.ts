import { useEffect, useState } from 'react'
import { apiFetch } from '@/api/client'
import type { DailyPracticeState } from '@/components/ui/DailyPracticeProgress'
import type { DailyPlanModuleKey, JLPTLevel, ModuleStat, UserStats } from '@/types/api'

type PracticeCounters = Pick<ModuleStat, 'daily_goal' | 'today_completed'> & { jlpt_levels?: JLPTLevel[] }

export function useDailyPracticeProgress(module: DailyPlanModuleKey, enabled = true) {
  const [result, setResult] = useState<{ progress: DailyPracticeState; jlptLevels?: JLPTLevel[] }>({ progress: { status: 'loading' } })
  const [revision, setRevision] = useState(0)

  useEffect(() => {
    if (!enabled) return
    const controller = new AbortController()
    const request: Promise<PracticeCounters | undefined> = module === 'translation'
      ? apiFetch<PracticeCounters>('GET', '/api/v1/translation/progress', undefined, controller.signal)
      : apiFetch<UserStats>('GET', '/api/v1/users/stats', undefined, controller.signal).then(stats => stats?.modules?.[module])
    request
      .then((item) => {
        if (controller.signal.aborted) return
        setResult(item ? {
          progress: { status: 'ready', completed: item.today_completed, goal: item.daily_goal, scope: item.jlpt_levels?.join(' / ') },
          jlptLevels: item.jlpt_levels,
        } : { progress: { status: 'error' } })
      })
      .catch(() => {
        if (!controller.signal.aborted) setResult({ progress: { status: 'error' } })
      })
    return () => controller.abort()
  }, [module, enabled, revision])

  return { ...result, refreshProgress: () => setRevision((value) => value + 1) }
}
