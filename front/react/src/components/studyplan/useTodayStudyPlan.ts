import { useCallback, useEffect, useRef, useState } from 'react'
import { getTodayStudyPlan, setStudyTaskCompleted, type TodayStudyPlan } from '@/api/studyplan'

export function useTodayStudyPlan(refreshKey = 0) {
  const [plan, setPlan] = useState<TodayStudyPlan | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [pendingTask, setPendingTask] = useState<string | null>(null)
  const [revision, setRevision] = useState(0)
  const generation = useRef(0)
  const mutation = useRef<AbortController | null>(null)
  const reload = useCallback(() => setRevision(value => value + 1), [])

  useEffect(() => {
    const controller = new AbortController()
    generation.current += 1
    mutation.current?.abort()
    setPendingTask(null)
    setLoading(true)
    setError('')
    getTodayStudyPlan(controller.signal)
      .then(result => { if (!controller.signal.aborted) setPlan(result) })
      .catch(err => { if (!controller.signal.aborted) setError(err instanceof Error ? err.message : '今日任务读取失败') })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => { controller.abort(); mutation.current?.abort() }
  }, [refreshKey, revision])

  useEffect(() => {
    const onFocus = () => reload()
    const onVisible = () => { if (document.visibilityState === 'visible') reload() }
    const timer = window.setInterval(() => {
      if (plan?.has_plan) {
        try {
          if (new Intl.DateTimeFormat('sv-SE', { timeZone: plan.timezone }).format(new Date()) !== plan.date) reload()
        } catch (err) { console.error('[study-plan] invalid plan timezone', err) }
      }
    }, 60_000)
    window.addEventListener('focus', onFocus)
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      window.clearInterval(timer)
      window.removeEventListener('focus', onFocus)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [plan, reload])

  const toggle = async (slotID: string, taskID: string, completed: boolean) => {
    if (!plan || pendingTask || loading) return
    const requestGeneration = generation.current
    const controller = new AbortController()
    mutation.current = controller
    setPendingTask(`${slotID}/${taskID}`)
    setError('')
    try {
      const result = await setStudyTaskCompleted(plan.date, slotID, taskID, completed, controller.signal)
      if (!controller.signal.aborted && requestGeneration === generation.current) setPlan(result)
    } catch (err) {
      if (!controller.signal.aborted && requestGeneration === generation.current) setError(err instanceof Error ? err.message : '打卡保存失败，请重试')
    } finally {
      if (!controller.signal.aborted && requestGeneration === generation.current) setPendingTask(null)
    }
  }

  return { plan, loading, error, pendingTask, toggle, reload }
}
