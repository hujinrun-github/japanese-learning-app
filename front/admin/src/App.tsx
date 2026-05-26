import { Routes, Route, Navigate, useLocation } from 'react-router-dom'
import Layout from './components/Layout/Layout'
import LoginPage from './pages/Login/Login'
import WordsPage from './pages/Words/Words'
import GrammarPage from './pages/Grammar/Grammar'
import SpeakingPage from './pages/Speaking/Speaking'
import WritingPage from './pages/Writing/Writing'

function PlaceholderPage({ title }: { title: string }) {
  return (
    <div>
      <h2>{title}</h2>
      <p>This page is under construction.</p>
    </div>
  )
}

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
        <Route path="translation" element={<PlaceholderPage title="Translation" />} />
        <Route path="users" element={<PlaceholderPage title="Users" />} />
        <Route path="records" element={<PlaceholderPage title="Records" />} />
      </Route>
    </Routes>
  )
}
