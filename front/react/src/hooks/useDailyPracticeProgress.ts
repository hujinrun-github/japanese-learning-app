import { useEffect, useState } from 'react'
import { apiFetch } from '@/api/client'
import type { DailyPracticeState } from '@/components/ui/DailyPracticeProgress'
import type { DailyPlanModuleKey, UserStats } from '@/types/api'

export function useDailyPracticeProgress(module: DailyPlanModuleKey) {
  const [progress, setProgress] = useState<DailyPracticeState>({ status: 'loading' })
  const [revision, setRevision] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    apiFetch<UserStats>('GET', '/api/v1/users/stats', undefined, controller.signal)
      .then((stats) => {
        if (controller.signal.aborted) return
        const item = stats?.modules?.[module]
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
