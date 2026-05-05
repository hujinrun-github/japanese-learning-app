import styles from './ProgressBar.module.css'

interface ProgressBarProps {
  value: number // 0–100
  label?: string
}

export function ProgressBar({ value, label }: ProgressBarProps) {
  const clamped = Math.max(0, Math.min(100, value))
  const colorClass = clamped >= 80 ? styles.high : clamped >= 40 ? styles.mid : styles.low

  return (
    <div className={styles.wrapper}>
      {label && <span className={styles.label}>{label}</span>}
      <div className={styles.track} role="progressbar" aria-valuenow={clamped} aria-valuemin={0} aria-valuemax={100}>
        <div className={`${styles.fill} ${colorClass}`} style={{ width: `${clamped}%` }} />
      </div>
    </div>
  )
}
