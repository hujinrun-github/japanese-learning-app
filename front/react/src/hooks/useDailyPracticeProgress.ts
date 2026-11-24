import { useEffect, useState } from 'react'
import { apiFetch } from '@/api/client'
import type { DailyPracticeState } from '@/components/ui/DailyPracticeProgress'
import type { DailyPlanModuleKey, ModuleStat, UserStats } from '@/types/api'

export function useDailyPracticeProgress(module: DailyPlanModuleKey) {
  const [progress, setProgress] = useState<DailyPracticeState>({ status: 'loading' })
  const [revision, setRevision] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    const request = module === 'translation'
      ? apiFetch<Pick<ModuleStat, 'daily_goal' | 'today_completed'>>('GET', '/api/v1/translation/progress', undefined, controller.signal)
      : apiFetch<UserStats>('GET', '/api/v1/users/stats', undefined, controller.signal).then(stats => stats?.modules?.[module])
    request
      .then((item) => {
        if (controller.signal.aborted) return
        setProgress(item
          ? { status: 'ready', completed: item.today_completed, goal: item.daily_goal }
          : { status: 'error' })
      })
      .catch(() => {
        if (!controller.signal.aborted) setProgress({ status: 'error' })
      })
    return () => controller.abort()
  }, [module, revision])

  return { progress, refreshProgress: () => setRevision((value) => value + 1) }
}
