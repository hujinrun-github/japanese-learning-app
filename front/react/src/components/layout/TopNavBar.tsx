import { useState, useRef, useEffect } from 'react'
import { NavLink, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/contexts/AuthContext'
import { LanguageSwitcher } from '@/components/ui/LanguageSwitcher'
import { DESKTOP_NAV_ITEMS } from './navigation'
import styles from './TopNavBar.module.css'

export function TopNavBar() {
  const { user, logout } = useAuth()
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [])

  return (
    <header className={styles.header}>
      <div className={styles.inner}>
        <NavLink to="/" className={styles.logo} aria-label={t('nav.home')}>
          <span aria-hidden="true">🇯🇵</span>
          <span className={styles.logoText}>{t('common.appName')}</span>
        </NavLink>

        <nav className={styles.nav}>
          {DESKTOP_NAV_ITEMS.filter((item) => item.to !== '/settings').map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) => `${styles.link} ${isActive ? styles.active : ''}`}
            >
              {t(item.labelKey)}
            </NavLink>
          ))}
        </nav>

        <div className={styles.user}>
          <div className={styles.desktopLanguage}><LanguageSwitcher /></div>
          <div className={styles.userMenu} ref={menuRef}>
            <button
              className={styles.userIcon}
              onClick={() => setMenuOpen(!menuOpen)}
              title={t('nav.settings')}
              aria-label={t('nav.accountMenu')}
              aria-expanded={menuOpen}
            >
              👤
            </button>
            {menuOpen && (
              <div className={styles.dropdown}>
                <button className={styles.dropdownItem} onClick={() => { navigate('/settings'); setMenuOpen(false) }}>
                  ⚙ {t('nav.settings')}
                </button>
                <button className={styles.dropdownItem} onClick={() => { logout(); setMenuOpen(false) }}>
                  ➡ {t('nav.logout')}
                </button>
              </div>
            )}
          </div>
          {user && <span className={styles.userName}>{user.name}</span>}
        </div>
      </div>
    </header>
  )
}
