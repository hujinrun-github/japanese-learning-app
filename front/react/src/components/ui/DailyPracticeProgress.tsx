import { useTranslation } from 'react-i18next'
import styles from './DailyPracticeProgress.module.css'

export type DailyPracticeState =
  | { status: 'loading' | 'error' | 'notScheduled' | 'taskUnavailable' }
  | { status: 'ready'; completed: number; goal: number; scope?: string; source?: 'plan' | 'habit' }

export function DailyPracticeProgress({ progress }: { progress: DailyPracticeState }) {
  const { t } = useTranslation()
  const label = progress.status === 'ready'
    ? t(progress.source ? `common.dailyProgress.${progress.source}`
      : progress.scope ? 'common.dailyProgress.scoped' : 'home.todayProgress', {
      completed: progress.completed, goal: progress.goal, scope: progress.scope,
    })
    : t(`common.dailyProgress.${progress.status}`)

  return <p className={styles.progress} role="status">{label}</p>
}
