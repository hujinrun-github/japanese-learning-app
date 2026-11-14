import { useTranslation } from 'react-i18next'
import styles from './DailyPracticeProgress.module.css'

export type DailyPracticeState =
  | { status: 'loading' | 'error' }
  | { status: 'ready'; completed: number; goal: number }

export function DailyPracticeProgress({ progress }: { progress: DailyPracticeState }) {
  const { t } = useTranslation()
  const label = progress.status === 'ready'
    ? t('home.todayProgress', { completed: progress.completed, goal: progress.goal })
    : t(`common.dailyProgress.${progress.status}`)

  return <p className={styles.progress} role="status">{label}</p>
}
