import { Routes, Route, Navigate, useLocation } from 'react-router-dom'
import Layout from './components/Layout/Layout'
import LoginPage from './pages/Login/Login'
import WordsPage from './pages/Words/Words'
import GrammarPage from './pages/Grammar/Grammar'
import SpeakingPage from './pages/Speaking/Speaking'
import WritingPage from './pages/Writing/Writing'
import TranslationPage from './pages/Translation/Translation'
import UsersPage from './pages/Users/Users'
import RecordsPage from './pages/Records/Records'
import ShadowingMaterialsPage from './pages/ShadowingMaterials/ShadowingMaterials'

function AuthGuard({ children }: { children: React.ReactNode }) {
  const token = sessionStorage.getItem('admin_token')
  const location = useLocation()

  if (!token) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }
  return <>{children}</>
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/"
        element={
          <AuthGuard>
            <Layout />
          </AuthGuard>
        }
      >
        <Route index element={<Navigate to="/words" replace />} />
        <Route path="words" element={<WordsPage />} />
        <Route path="grammar" element={<GrammarPage />} />
        <Route path="speaking" element={<SpeakingPage />} />
        <Route path="writing" element={<WritingPage />} />
        <Route path="translation" element={<TranslationPage />} />
        <Route path="shadowing-materials" element={<ShadowingMaterialsPage />} />
        <Route path="users" element={<UsersPage />} />
        <Route path="records" element={<RecordsPage />} />
      </Route>
    </Routes>
  )
}
