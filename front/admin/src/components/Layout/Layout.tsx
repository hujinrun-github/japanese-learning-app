import { useState } from 'react'
import { Outlet, NavLink, useNavigate } from 'react-router-dom'
import styles from './Layout.module.css'

const NAV_ITEMS = [
  { to: '/words',       label: 'Words',        icon: '📝' },
  { to: '/grammar',     label: 'Grammar',      icon: '📐' },
  { to: '/speaking',    label: 'Speaking',     icon: '🎙' },
  { to: '/writing',     label: 'Writing',      icon: '✍️' },
  { to: '/translation', label: 'Translation',  icon: '🔄' },
  { to: '/users',       label: 'Users',        icon: '👥' },
  { to: '/records',     label: 'Records',      icon: '📋' },
] as const

export default function Layout() {
  const navigate = useNavigate()
  const [collapsed, setCollapsed] = useState(false)

  function handleLogout() {
    sessionStorage.removeItem('admin_token')
    navigate('/login')
  }

  return (
    <div className={`${styles.layout} ${collapsed ? styles.collapsed : ''}`}>
      <aside className={styles.sidebar}>
        <div className={styles.brand}>
          <span className={styles.brandIcon}>🎌</span>
          <span className={styles.brandName}>Admin Panel</span>
          <span className={styles.brandSub}>日本語学習</span>
        </div>
        <nav className={styles.nav}>
          {NAV_ITEMS.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              title={collapsed ? item.label : undefined}
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
              className={styles.collapseBtn}
              onClick={() => setCollapsed(!collapsed)}
              title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            >
              {collapsed ? '☰' : '◁'}
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
