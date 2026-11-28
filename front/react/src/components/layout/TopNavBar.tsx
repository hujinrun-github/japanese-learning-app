import { useState, useRef, useEffect, type FocusEvent } from 'react'
import { matchPath, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/contexts/AuthContext'
import { LanguageSwitcher } from '@/components/ui/LanguageSwitcher'
import { FloatingSearchButton } from '@/pages/search/ContextualSearch'
import { ACCOUNT_NAV_ITEMS, DESKTOP_NAV_GROUPS, DESKTOP_PRIMARY_ITEMS, activeDesktopGroup, type DesktopGroupID } from './navigation'
import styles from './TopNavBar.module.css'

type Menu = DesktopGroupID | 'account'

export function TopNavBar() {
  const { user, logout } = useAuth()
  const { t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const [openMenu, setOpenMenu] = useState<Menu | null>(null)
  const openTrigger = useRef<HTMLButtonElement | null>(null)
  const activeGroup = activeDesktopGroup(location.pathname)
  const isSearchPage = !!matchPath('/search/*', location.pathname)

  useEffect(() => { setOpenMenu(null) }, [location.key])

  useEffect(() => {
    if (!openMenu) return
    function handleOutsideClick(event: PointerEvent) {
      if (!openTrigger.current?.parentElement?.contains(event.target as Node)) setOpenMenu(null)
    }
    function handleEscape(event: KeyboardEvent) {
      if (event.key !== 'Escape') return
      event.preventDefault()
      openTrigger.current?.focus()
      setOpenMenu(null)
    }
    document.addEventListener('pointerdown', handleOutsideClick)
    document.addEventListener('keydown', handleEscape)
    return () => {
      document.removeEventListener('pointerdown', handleOutsideClick)
      document.removeEventListener('keydown', handleEscape)
    }
  }, [openMenu])

  function toggleMenu(menu: Menu, trigger: HTMLButtonElement) {
    openTrigger.current = trigger
    setOpenMenu(current => current === menu ? null : menu)
  }

  function dismissOnBlur(event: FocusEvent<HTMLDivElement>) {
    if (event.relatedTarget && !event.currentTarget.contains(event.relatedTarget)) setOpenMenu(null)
  }

  function navigateFromMenu(path: string) {
    setOpenMenu(null)
    navigate(path)
  }

  return (
    <header className={styles.header}>
      <div className={styles.inner}>
        <NavLink to="/" className={styles.logo} aria-label={t('nav.home')}>
          <span aria-hidden="true">🇯🇵</span>
          <span className={styles.logoText}>{t('common.appName')}</span>
        </NavLink>

        <nav className={styles.nav} aria-label={t('nav.mainNav')}>
          {DESKTOP_PRIMARY_ITEMS.map(item => item.to === '/search' ? (
            <FloatingSearchButton key={item.to}
              className={`${styles.link} ${styles.trigger} ${isSearchPage ? styles.active : ''}`}
              onOpen={() => setOpenMenu(null)}>
              <span aria-hidden="true">{item.icon}</span>{t(item.labelKey)}
            </FloatingSearchButton>
          ) : (
            <NavLink key={item.to} to={item.to} end={item.end}
              onClick={() => setOpenMenu(null)}
              className={({ isActive }) => `${styles.link} ${isActive ? styles.active : ''}`}>
              {t(item.labelKey)}
            </NavLink>
          ))}
          {DESKTOP_NAV_GROUPS.map(group => {
            const menuID = `desktop-nav-${group.id}`
            return (
              <div className={styles.navGroup} key={group.id} onBlur={dismissOnBlur}>
                <button type="button" className={`${styles.link} ${styles.trigger} ${activeGroup === group.id ? styles.active : ''}`}
                  aria-expanded={openMenu === group.id} aria-controls={menuID}
                  aria-current={activeGroup === group.id ? 'true' : undefined}
                  onClick={event => toggleMenu(group.id, event.currentTarget)}>
                  {t(group.labelKey)}
                  <span className={styles.chevron} aria-hidden="true" />
                </button>
                <div className={`${styles.dropdown} ${styles.navDropdown}`} id={menuID} hidden={openMenu !== group.id}>
                  {group.items.map(item => (
                    <NavLink key={item.to} to={item.to} end={item.end}
                      onClick={() => setOpenMenu(null)}
                      className={({ isActive }) => `${styles.dropdownItem} ${isActive ? styles.active : ''}`}>
                      <span aria-hidden="true">{item.icon}</span>
                      {t(item.labelKey)}
                    </NavLink>
                  ))}
                </div>
              </div>
            )
          })}
        </nav>

        <div className={styles.user}>
          <div className={styles.desktopLanguage}><LanguageSwitcher /></div>
          <div className={styles.userMenu} onBlur={dismissOnBlur}>
            <button
              type="button"
              className={styles.userIcon}
              onClick={event => toggleMenu('account', event.currentTarget)}
              title={t('nav.settings')}
              aria-label={t('nav.accountMenu')}
              aria-expanded={openMenu === 'account'}
              aria-controls="desktop-account-menu"
            >
              👤
            </button>
            <div className={styles.dropdown} id="desktop-account-menu" hidden={openMenu !== 'account'}>
              <button type="button" className={styles.dropdownItem} onClick={() => navigateFromMenu('/')}>🏠 {t('nav.home')}</button>
              {ACCOUNT_NAV_ITEMS.map(item => (
                <button type="button" key={item.to} className={styles.dropdownItem} onClick={() => navigateFromMenu(item.to)}>
                  <span aria-hidden="true">{item.icon}</span> {t(item.labelKey)}
                </button>
              ))}
              <button type="button" className={styles.dropdownItem} onClick={() => { setOpenMenu(null); logout() }}>
                ➡ {t('nav.logout')}
              </button>
            </div>
          </div>
          {user && <span className={styles.userName}>{user.name}</span>}
        </div>
      </div>
    </header>
  )
}
