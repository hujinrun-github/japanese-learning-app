import type { ReactNode } from 'react'
import styles from './Card.module.css'

interface CardProps {
  children: ReactNode
  hoverable?: boolean
  onClick?: () => void
  padding?: 'sm' | 'md' | 'lg'
  className?: string
}

export function Card({ children, hoverable = false, onClick, padding = 'md', className }: CardProps) {
  return (
    <div
      className={`${styles.card} ${styles[padding]} ${hoverable ? styles.hoverable : ''} ${className ?? ''}`}
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={onClick ? (e) => { if (e.key === 'Enter' || e.key === ' ') onClick() } : undefined}
    >
      {children}
    </div>
  )
}
