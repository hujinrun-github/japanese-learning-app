import { lazy, Suspense } from 'react'
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
import { TranslationListPage } from '@/pages/translation/TranslationListPage'
import { TranslationPracticePage } from '@/pages/translation/TranslationPracticePage'
import { WritingTopicListPage } from '@/pages/writing/WritingTopicListPage'
import { WritingQueuePage } from '@/pages/writing/WritingQueuePage'
import { WritingQuestionsPage } from '@/pages/writing/WritingQuestionsPage'
import { LessonListPage } from '@/pages/lesson/LessonListPage'
import { LessonDetailPage } from '@/pages/lesson/LessonDetailPage'
import { ShadowingPage } from '@/pages/shadowing/ShadowingPage'
import { NoteListPage } from '@/pages/note/NoteListPage'
import { NoteEditPage } from '@/pages/note/NoteEditPage'
import { NoteDetailPage } from '@/pages/note/NoteDetailPage'
import { StudyPlanPage } from '@/pages/studyplan/StudyPlanPage'
import SettingsPage from '@/pages/settings/SettingsPage'
import { ErrataHistoryPage } from '@/pages/errata/ErrataHistoryPage'
import { FavoritesPage } from '@/pages/favorites/FavoritesPage'
import { MistakesPage } from '@/pages/mistakes/MistakesPage'
import { ListeningPage } from '@/pages/listening/ListeningPage'
import { ListeningTrainingPage } from '@/pages/listening/ListeningTrainingPage'
import { Spinner } from '@/components/ui/Spinner'

const SearchPage = lazy(() => import('@/pages/search/SearchPage'))
const WordEntryPage = lazy(() => import('@/pages/search/WordEntryPage'))

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
            <Route path="/search" element={<Suspense fallback={<Spinner />}><SearchPage /></Suspense>} />
            <Route path="/search/words/:id" element={<Suspense fallback={<Spinner />}><WordEntryPage /></Suspense>} />
            <Route path="/words/review" element={<WordReviewPage />} />
            <Route path="/grammar" element={<GrammarListPage />} />
            <Route path="/grammar/:id" element={<GrammarDetailPage />} />
            <Route path="/speaking" element={<SpeakingPage />} />
            <Route path="/listening" element={<ListeningPage />} />
            <Route path="/listening/:id" element={<ListeningTrainingPage />} />
            <Route path="/writing" element={<WritingTopicListPage />} />
            <Route path="/writing/practice" element={<WritingQueuePage />} />
            <Route path="/writing/questions" element={<WritingQuestionsPage />} />
            <Route path="/lesson" element={<LessonListPage />} />
            <Route path="/lesson/:id" element={<LessonDetailPage />} />
            <Route path="/lesson/:id/shadowing" element={<ShadowingPage />} />
            <Route path="/notes" element={<NoteListPage />} />
            <Route path="/notes/new" element={<NoteEditPage />} />
            <Route path="/notes/:id" element={<NoteDetailPage />} />
            <Route path="/translation" element={<TranslationListPage />} />
            <Route path="/translation/practice" element={<TranslationPracticePage />}>
              <Route path="grammar" element={<GrammarListPage fromTranslation />} />
              <Route path="grammar/:id" element={<GrammarDetailPage fromTranslation />} />
            </Route>
            <Route path="/study-plan" element={<StudyPlanPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/errata" element={<ErrataHistoryPage />} />
            <Route path="/favorites" element={<FavoritesPage />} />
            <Route path="/mistakes" element={<MistakesPage />} />
          </Route>

          {/* Fallback */}
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  )
}
