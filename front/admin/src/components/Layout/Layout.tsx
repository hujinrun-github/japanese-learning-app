import { useEffect, useState } from 'react'
import { Outlet, NavLink, useLocation, useNavigate } from 'react-router-dom'
import styles from './Layout.module.css'

const NAV_ITEMS = [
  { to: '/words', label: 'Words', icon: '📘' },
  { to: '/grammar', label: 'Grammar', icon: '📚' },
  { to: '/speaking', label: 'Speaking', icon: '🎙️' },
  { to: '/writing', label: 'Writing', icon: '✍️' },
  { to: '/translation', label: 'Translation', icon: '🔁' },
  { to: '/shadowing-materials', label: 'Shadowing', icon: '🎬' },
  { to: '/users', label: 'Users', icon: '👥' },
  { to: '/records', label: 'Records', icon: '📊' },
] as const

export default function Layout() {
  const navigate = useNavigate()
  const location = useLocation()
  const [collapsed, setCollapsed] = useState(false)
  const [mobileOpen, setMobileOpen] = useState(false)

  useEffect(() => {
    setMobileOpen(false)
  }, [location.pathname])

  function handleLogout() {
    sessionStorage.removeItem('admin_token')
    navigate('/login')
  }

  return (
    <div className={`${styles.layout} ${collapsed ? styles.collapsed : ''} ${mobileOpen ? styles.mobileOpen : ''}`}>
      <div
        className={styles.backdrop}
        onClick={() => setMobileOpen(false)}
        aria-hidden="true"
      />
      <aside className={styles.sidebar} aria-label="Admin navigation">
        <div className={styles.brand}>
          <span className={styles.brandIcon}>🎌</span>
          <span className={styles.brandName}>Admin Panel</span>
          <span className={styles.brandSub}>Japanese Learning</span>
          <button
            className={styles.sidebarCloseBtn}
            onClick={() => setMobileOpen(false)}
            aria-label="Close navigation"
          >
            ✕
          </button>
        </div>
        <nav className={styles.nav}>
          {NAV_ITEMS.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              title={collapsed ? item.label : undefined}
              onClick={() => setMobileOpen(false)}
              className={({ isActive }) =>
                isActive ? `${styles.navItem} ${styles.active}` : styles.navItem
              }
            >
              <span className={styles.navIcon}>{item.icon}</span>
              <span className={styles.navLabel}>{item.label}</span>
            </NavLink>
          ))}
        </nav>
        <div className={styles.navFooter}>
          <span className={styles.footerText}>v0.1 · admin</span>
        </div>
      </aside>
      <div className={styles.main}>
        <header className={styles.topbar}>
          <div className={styles.topLeft}>
            <button
              className={`${styles.collapseBtn} ${styles.desktopCollapseBtn}`}
              onClick={() => setCollapsed(!collapsed)}
              title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            >
              {collapsed ? '›' : '‹'}
            </button>
            <button
              className={`${styles.collapseBtn} ${styles.mobileMenuBtn}`}
              onClick={() => setMobileOpen(true)}
              title="Open navigation"
              aria-label="Open navigation"
              aria-expanded={mobileOpen}
            >
              ☰
            </button>
            <h1 className={styles.topTitle}>Admin Panel</h1>
            <span className={styles.topBreadcrumb}>/ Japanese Learning</span>
          </div>
          <button className={styles.logoutBtn} onClick={handleLogout}>
            Logout
          </button>
        </header>
        <main className={styles.content}>
          <Outlet />
        </main>
      </div>
    </div>
  )
}
