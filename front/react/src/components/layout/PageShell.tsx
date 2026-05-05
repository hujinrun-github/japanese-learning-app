import { useEffect, type ReactNode } from 'react'
import { TopNavBar } from './TopNavBar'
import { BottomTabBar } from './BottomTabBar'
import styles from './PageShell.module.css'

interface PageShellProps {
  children: ReactNode
  title?: string
}

export function PageShell({ children, title }: PageShellProps) {
  useEffect(() => {
    document.title = title ? `${title} — 日本語学習` : '日本語学習'
  }, [title])

  return (
    <div className={styles.shell}>
      <TopNavBar />
      <main className={styles.main}>{children}</main>
      <BottomTabBar />
    </div>
  )
}
