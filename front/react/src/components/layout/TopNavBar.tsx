import { NavLink } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/contexts/AuthContext'
import { LanguageSwitcher } from '@/components/ui/LanguageSwitcher'
import styles from './TopNavBar.module.css'

export function TopNavBar() {
  const { user, logout } = useAuth()
  const { t } = useTranslation()

  return (
    <header className={styles.header}>
      <div className={styles.inner}>
        <NavLink to="/" className={styles.logo}>
          🇯🇵 {t('common.appName')}
        </NavLink>

        <nav className={styles.nav}>
          <NavLink to="/" className={({ isActive }) => `${styles.link} ${isActive ? styles.active : ''}`} end>
            {t('nav.home')}
          </NavLink>
          <NavLink to="/words/review" className={({ isActive }) => `${styles.link} ${isActive ? styles.active : ''}`}>
            {t('nav.words')}
          </NavLink>
          <NavLink to="/grammar" className={({ isActive }) => `${styles.link} ${isActive ? styles.active : ''}`}>
            {t('nav.grammar')}
          </NavLink>
          <NavLink to="/speaking" className={({ isActive }) => `${styles.link} ${isActive ? styles.active : ''}`}>
            {t('nav.speaking')}
          </NavLink>
          <NavLink to="/writing" className={({ isActive }) => `${styles.link} ${isActive ? styles.active : ''}`}>
            {t('nav.writing')}
          </NavLink>
          <NavLink to="/notes" className={({ isActive }) => `${styles.link} ${isActive ? styles.active : ''}`}>
            {t('nav.notes')}
          </NavLink>
        </nav>

        <div className={styles.user}>
          <LanguageSwitcher />
          {user && <span className={styles.userName}>{user.name}</span>}
          <button className={styles.logoutBtn} onClick={logout} title={t('nav.logout')}>
            ⏻
          </button>
        </div>
      </div>
    </header>
  )
}
