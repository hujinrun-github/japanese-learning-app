import { Navigate, Outlet } from 'react-router-dom'
import { useAuth } from '@/contexts/AuthContext'
import { PageShell } from '@/components/layout/PageShell'
import { Spinner } from '@/components/ui/Spinner'

export function ProtectedLayout() {
  const { isAuthenticated, token } = useAuth()

  // Token is still being restored from localStorage (initial render)
  if (token === null && localStorage.getItem('token')) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', marginTop: '4rem' }}>
        <Spinner size="lg" />
      </div>
    )
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />
  }

  return (
    <PageShell>
      <Outlet />
    </PageShell>
  )
}
