import { useTranslation } from 'react-i18next'
import styles from './Spinner.module.css'

interface SpinnerProps {
  size?: 'sm' | 'md' | 'lg'
}

export function Spinner({ size = 'md' }: SpinnerProps) {
  const { t } = useTranslation()
  const label = t('common.loading')

  return (
    <span className={`${styles.spinner} ${styles[size]}`} role="status" aria-label={label}>
      <span className="visually-hidden">{label}</span>
    </span>
  )
}
