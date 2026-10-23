import { useEffect, useState } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/contexts/AuthContext'
import { LanguageSwitcher } from '@/components/ui/LanguageSwitcher'
import { MOBILE_MORE_ITEMS, MOBILE_PRIMARY_ITEMS } from './navigation'
import styles from './BottomTabBar.module.css'

export function BottomTabBar() {
  const { t } = useTranslation()
  const { user, logout } = useAuth()
  const { pathname } = useLocation()
  const [moreOpen, setMoreOpen] = useState(false)
  const moreActive = MOBILE_MORE_ITEMS.some((item) => pathname.startsWith(item.to))

  useEffect(() => {
    setMoreOpen(false)
  }, [pathname])

  useEffect(() => {
    if (!moreOpen) return
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMoreOpen(false)
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [moreOpen])

  return (
    <>
      {moreOpen && (
        <div className={styles.overlay} onClick={() => setMoreOpen(false)}>
          <section
            id="mobile-more-menu"
            className={styles.sheet}
            role="dialog"
            aria-modal="true"
            aria-label={t('nav.more')}
            onClick={(event) => event.stopPropagation()}
          >
            <div className={styles.sheetHandle} aria-hidden="true" />
            <div className={styles.sheetHeader}>
              <div>
                <strong>{user?.name || user?.email || t('common.appName')}</strong>
                <span>{t('nav.moreDescription')}</span>
              </div>
              <button className={styles.closeButton} onClick={() => setMoreOpen(false)} aria-label={t('nav.closeMenu')}>
                ✕
              </button>
            </div>
            <div className={styles.moreGrid}>
              {MOBILE_MORE_ITEMS.map((item) => (
                <NavLink key={item.to} to={item.to} className={styles.moreItem}>
                  <span className={styles.moreIcon}>{item.icon}</span>
                  <span>{t(item.labelKey)}</span>
                </NavLink>
              ))}
            </div>
            <div className={styles.sheetFooter}>
              <LanguageSwitcher />
              <button className={styles.logoutButton} onClick={logout}>➡️ {t('nav.logout')}</button>
            </div>
          </section>
        </div>
      )}

      <nav className={styles.nav} aria-label={t('nav.mainNav')}>
        {MOBILE_PRIMARY_ITEMS.map((tab) => (
          <NavLink
            key={tab.to}
            to={tab.to}
            end={tab.end}
            className={({ isActive }) => `${styles.tab} ${isActive ? styles.active : ''}`}
          >
            <span className={styles.icon}>{tab.icon}</span>
            <span className={styles.label}>{t(tab.labelKey)}</span>
          </NavLink>
        ))}
        <button
          className={`${styles.tab} ${moreActive || moreOpen ? styles.active : ''}`}
          onClick={() => setMoreOpen((open) => !open)}
          aria-expanded={moreOpen}
          aria-controls="mobile-more-menu"
        >
          <span className={styles.icon}>☰</span>
          <span className={styles.label}>{t('nav.more')}</span>
        </button>
      </nav>
    </>
  )
}
