import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import type { Note, NoteType, PaginatedNotes } from '@/types/api'
import styles from './NoteListPage.module.css'

const TYPE_FILTERS: { key: NoteType | 'all'; labelKey: string }[] = [
  { key: 'all', labelKey: 'notes.filterAll' },
  { key: 'word', labelKey: 'notes.filterWord' },
  { key: 'grammar', labelKey: 'notes.filterGrammar' },
  { key: 'sentence', labelKey: 'notes.filterSentence' },
]

const TYPE_COLORS: Record<string, string> = {
  word: 'var(--color-primary)',
  grammar: 'var(--color-warning)',
  sentence: 'var(--color-success)',
}

const PAGE_SIZE = 20

const NOTE_EMPTY: PaginatedNotes = { items: [], total: 0, page: 1, size: PAGE_SIZE }

export function NoteListPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [typeFilter, setTypeFilter] = useState<NoteType | 'all'>('all')
  const [data, setData] = useState<PaginatedNotes>(NOTE_EMPTY)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError('')
    const params = new URLSearchParams()
    if (typeFilter !== 'all') params.set('type', typeFilter)
    params.set('page', String(page))
    params.set('size', String(PAGE_SIZE))

    apiFetch<PaginatedNotes>('GET', `/api/v1/notes?${params.toString()}`)
      .then((d) => {
        if (!cancelled) {
          setData(d && d.items ? d : NOTE_EMPTY)
        }
      })
      .catch((err: Error) => { if (!cancelled) { setError(err.message); setData(NOTE_EMPTY) } })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [typeFilter, page])

  const items = data?.items ?? []
  const totalPages = Math.max(1, Math.ceil((data?.total ?? 0) / PAGE_SIZE))
  const empty = !loading && items.length === 0

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>{t('notes.title')}</h1>

      {/* Type filter tabs */}
      <div className={styles.tabs}>
        {TYPE_FILTERS.map((f) => (
          <button
            key={f.key}
            className={`${styles.tab} ${typeFilter === f.key ? styles.tabActive : ''}`}
            onClick={() => { setTypeFilter(f.key); setPage(1) }}
          >
            {t(f.labelKey)}
          </button>
        ))}
      </div>

      {error && <p className={styles.error}>{error}</p>}
      {loading ? (
        <div className={styles.spinnerWrap}><Spinner size="md" /></div>
      ) : empty ? (
        <EmptyState icon="🗒️" title={t('notes.empty')} />
      ) : (
        <>
          <div className={styles.list}>
            {items.map((note) => (
              <NoteCard key={note.id} note={note} onClick={() => navigate(`/notes/${note.id}`)} />
            ))}
          </div>

          {/* Pagination */}
          {totalPages > 1 && (
            <div className={styles.pagination}>
              <button
                className={styles.pageBtn}
                disabled={page <= 1}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
              >
                &lt;
              </button>
              <span className={styles.pageInfo}>{page} / {totalPages}</span>
              <button
                className={styles.pageBtn}
                disabled={page >= totalPages}
                onClick={() => setPage((p) => p + 1)}
              >
                &gt;
              </button>
            </div>
          )}
        </>
      )}

      {/* FAB */}
      <button
        className={styles.fab}
        aria-label={t('notes.create')}
        onClick={() => navigate('/notes/new')}
      >
        +
      </button>
    </div>
  )
}

function NoteCard({ note, onClick }: { note: Note; onClick: () => void }) {
  return (
    <div className={styles.card} onClick={onClick} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter') onClick() }}>
      <div className={styles.cardHeader}>
        <span className={styles.typeDot} style={{ background: TYPE_COLORS[note.type] ?? 'var(--color-text-disabled)' }} />
        <span className={styles.cardTitle}>{note.title}</span>
      </div>
      {note.source_text && (
        <p className={styles.sourcePreview}>{note.source_text}</p>
      )}
      <div className={styles.cardFooter}>
        <div className={styles.tagRow}>
          {note.tags.slice(0, 4).map((tag) => (
            <span key={tag} className={styles.tag}>{tag}</span>
          ))}
          {note.tags.length > 4 && (
            <span className={styles.tagMore}>+{note.tags.length - 4}</span>
          )}
        </div>
        <span className={styles.date}>
          {new Date(note.updated_at).toLocaleDateString()}
        </span>
      </div>
    </div>
  )
}
