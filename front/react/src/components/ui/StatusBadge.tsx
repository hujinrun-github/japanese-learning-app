import { useTranslation } from 'react-i18next'
import styles from './StatusBadge.module.css'

export type StatusType = 'unlearned' | 'learning' | 'mastered' | 'pass' | 'needs_work'

interface StatusBadgeProps {
  status: StatusType
}

const STATUS_I18N_KEY: Record<StatusType, string> = {
  unlearned: 'status.unlearned',
  learning: 'status.learning',
  mastered: 'status.mastered',
  pass: 'status.pass',
  needs_work: 'status.needsWork',
}

export function StatusBadge({ status }: StatusBadgeProps) {
  const { t } = useTranslation()
  return (
    <span className={`${styles.badge} ${styles[status]}`} data-status={status}>
      {t(STATUS_I18N_KEY[status])}
    </span>
  )
}
