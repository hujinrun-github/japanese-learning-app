import type { JLPTLevel } from '@/types/api'
import styles from './Badge.module.css'

interface BadgeProps {
  level: JLPTLevel
  size?: 'sm' | 'md'
}

export function Badge({ level, size = 'md' }: BadgeProps) {
  return (
    <span className={`${styles.badge} ${styles[level]} ${styles[size]}`}>
      {level}
    </span>
  )
}
