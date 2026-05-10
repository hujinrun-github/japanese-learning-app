import { NavLink } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import styles from './BottomTabBar.module.css'

const TAB_CONFIG = [
  { to: '/',             key: 'nav.home',     icon: '🏠', end: true },
  { to: '/words/review', key: 'nav.words',    icon: '📖', end: false },
  { to: '/grammar',      key: 'nav.grammar',  icon: '📝', end: false },
  { to: '/speaking',     key: 'nav.speaking', icon: '🎙️', end: false },
  { to: '/writing',      key: 'nav.writing',  icon: '✏️', end: false },
  { to: '/notes',       key: 'nav.notes',    icon: '🗒️', end: false },
  { to: '/lesson',       key: 'nav.lesson',   icon: '📚', end: false },
]

export function BottomTabBar() {
  const { t } = useTranslation()

  return (
    <nav className={styles.nav} aria-label={t('nav.mainNav')}>
      {TAB_CONFIG.map((tab) => (
        <NavLink
          key={tab.to}
          to={tab.to}
          end={tab.end}
          className={({ isActive }) => `${styles.tab} ${isActive ? styles.active : ''}`}
        >
          <span className={styles.icon}>{tab.icon}</span>
          <span className={styles.label}>{t(tab.key)}</span>
        </NavLink>
      ))}
    </nav>
  )
}
