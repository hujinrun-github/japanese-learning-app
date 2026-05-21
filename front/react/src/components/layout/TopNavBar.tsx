import { useState, useRef, useEffect } from 'react'
import { NavLink, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/contexts/AuthContext'
import { LanguageSwitcher } from '@/components/ui/LanguageSwitcher'
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
          <NavLink to="/translation" className={({ isActive }) => `${styles.link} ${isActive ? styles.active : ''}`}>
            {t('nav.translation')}
          </NavLink>
        </nav>

        <div className={styles.user}>
          <LanguageSwitcher />
          <div className={styles.userMenu} ref={menuRef}>
            <button className={styles.userIcon} onClick={() => setMenuOpen(!menuOpen)} title={t('nav.settings')}>
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
