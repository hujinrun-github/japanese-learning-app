import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from '@/contexts/AuthContext'
import { ProtectedLayout } from '@/router/ProtectedLayout'
import { LoginPage } from '@/pages/auth/LoginPage'
import { RegisterPage } from '@/pages/auth/RegisterPage'
import { ForgotPasswordPage } from '@/pages/auth/ForgotPasswordPage'
import { ResetPasswordPage } from '@/pages/auth/ResetPasswordPage'
import { HomePage } from '@/pages/home/HomePage'
import { WordReviewPage } from '@/pages/word/WordReviewPage'
import { GrammarListPage } from '@/pages/grammar/GrammarListPage'
import { GrammarDetailPage } from '@/pages/grammar/GrammarDetailPage'
import { SpeakingPage } from '@/pages/speaking/SpeakingPage'
import { WritingQueuePage } from '@/pages/writing/WritingQueuePage'
import { LessonPage } from '@/pages/lesson/LessonPage'
import { NoteListPage } from '@/pages/note/NoteListPage'
import { NoteEditPage } from '@/pages/note/NoteEditPage'
import { NoteDetailPage } from '@/pages/note/NoteDetailPage'

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          {/* Public routes */}
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route path="/forgot-password" element={<ForgotPasswordPage />} />
          <Route path="/reset-password" element={<ResetPasswordPage />} />

          {/* Protected routes */}
          <Route element={<ProtectedLayout />}>
            <Route index element={<HomePage />} />
            <Route path="/words/review" element={<WordReviewPage />} />
            <Route path="/grammar" element={<GrammarListPage />} />
            <Route path="/grammar/:id" element={<GrammarDetailPage />} />
            <Route path="/speaking" element={<SpeakingPage />} />
            <Route path="/writing" element={<WritingQueuePage />} />
            <Route path="/lesson" element={<LessonPage />} />
            <Route path="/notes" element={<NoteListPage />} />
            <Route path="/notes/new" element={<NoteEditPage />} />
            <Route path="/notes/:id" element={<NoteDetailPage />} />
          </Route>

          {/* Fallback */}
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  )
}
