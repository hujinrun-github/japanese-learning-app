import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { getMistakes, setMistakeStatus } from '@/api/mistakes'
import type { Mistake, MistakeList, MistakeModule, MistakeStatus } from '@/types/api'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import styles from './MistakesPage.module.css'
import { mistakePageWithinTotal } from './mistakePagination'

const PAGE_SIZE = 20

type ModuleFilter = MistakeModule | 'all'
type StatusFilter = MistakeStatus | 'all'

const MODULE_TABS: { key: ModuleFilter; labelKey: string }[] = [
  { key: 'all', labelKey: 'mistakes.modules.all' },
  { key: 'word', labelKey: 'mistakes.modules.word' },
  { key: 'grammar', labelKey: 'mistakes.modules.grammar' },
  { key: 'writing', labelKey: 'mistakes.modules.writing' },
  { key: 'translation', labelKey: 'mistakes.modules.translation' },
]

const STATUS_TABS: { key: StatusFilter; labelKey: string }[] = [
  { key: 'unmastered', labelKey: 'mistakes.status.unmastered' },
  { key: 'mastered', labelKey: 'mistakes.status.mastered' },
  { key: 'all', labelKey: 'mistakes.status.all' },
]

const EMPTY: MistakeList = { items: [], total: 0, page: 1, size: PAGE_SIZE }

export function MistakesPage() {
  const { t } = useTranslation()
  const [moduleFilter, setModuleFilter] = useState<ModuleFilter>('all')
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('unmastered')
  const [page, setPage] = useState(1)
  const [attempt, setAttempt] = useState(0)
  const [data, setData] = useState<MistakeList>(EMPTY)
  const [loading, setLoading] = useState(true)
  const [loadFailed, setLoadFailed] = useState(false)
  const [actionFailed, setActionFailed] = useState(false)
  const [pendingId, setPendingId] = useState<number | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setLoadFailed(false)
    getMistakes({ module: moduleFilter, status: statusFilter, page, size: PAGE_SIZE })
      .then((d) => {
        if (cancelled) return
        const next = d && d.items ? d : EMPTY
        const validPage = mistakePageWithinTotal(page, next.total, next.size || PAGE_SIZE)
        if (validPage !== page) { setPage(validPage); return }
        setData(next)
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          console.error(err)
          setLoadFailed(true)
          setData(EMPTY)
        }
      })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [moduleFilter, statusFilter, page, attempt])

  const items = data.items
  const total = data.total
  const totalPages = Math.max(1, Math.ceil(total / (data.size || PAGE_SIZE)))
  const empty = !loading && !loadFailed && items.length === 0

  async function handleStatusChange(item: Mistake) {
    const next: MistakeStatus = item.status === 'mastered' ? 'unmastered' : 'mastered'
    setPendingId(item.id)
    setActionFailed(false)
    try {
      await setMistakeStatus(item.id, next)
      setAttempt(value => value + 1)
    } catch (err) {
      console.error(err)
      setActionFailed(true)
    } finally {
      setPendingId(null)
    }
  }

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <h1 className={styles.title}>{t('mistakes.title')}</h1>
        <p className={styles.subtitle}>{t('mistakes.subtitle')}</p>
        {!loading && !loadFailed && (
          <p className={styles.total}>{t('mistakes.total', { total })}</p>
        )}
      </header>

      <div className={styles.filterGroup}>
        <div className={styles.tabs}>
          {MODULE_TABS.map((f) => (
            <button
              key={f.key}
              className={`${styles.tab} ${moduleFilter === f.key ? styles.tabActive : ''}`}
              onClick={() => { setModuleFilter(f.key); setPage(1) }}
            >
              {t(f.labelKey)}
            </button>
          ))}
        </div>
        <div className={styles.tabs}>
          {STATUS_TABS.map((f) => (
            <button
              key={f.key}
              className={`${styles.tab} ${statusFilter === f.key ? styles.tabActive : ''}`}
              onClick={() => { setStatusFilter(f.key); setPage(1) }}
            >
              {t(f.labelKey)}
            </button>
          ))}
        </div>
      </div>

      {actionFailed && <p className={styles.error} role="alert">{t('mistakes.actionError')}</p>}

      {loadFailed ? (
        <div className={styles.message}>
          <p className={styles.error} role="alert">{t('mistakes.error')}</p>
          <Button variant="secondary" onClick={() => setAttempt((a) => a + 1)}>{t('mistakes.retry')}</Button>
        </div>
      ) : loading ? (
        <div className={styles.spinnerWrap}><Spinner size="lg" /></div>
      ) : empty ? (
        <EmptyState icon="📕" title={t('mistakes.empty')} description={t('mistakes.emptyDesc')} />
      ) : (
        <>
          <div className={styles.list}>
            {items.map((item) => (
              <article className={styles.card} key={item.id}>
                <div className={styles.cardHeader}>
                  <span className={styles.moduleTag}>{t(`mistakes.modules.${item.module}`)}</span>
                  <span className={styles.wrongCount}>{t('mistakes.wrongCount', { times: item.wrong_count })}</span>
                </div>
                <div className={styles.promptRow}>
                  <span className={styles.answerLabel}>{t('mistakes.prompt')}</span>
                  <p className={styles.prompt}>{item.prompt}</p>
                </div>
                <div className={styles.answers}>
                  <div className={styles.answerRow}>
                    <span className={styles.answerLabel}>{t('mistakes.yourAnswer')}</span>
                    <span className={`${styles.answerValue} ${styles.userAnswer}`}>{item.user_answer}</span>
                  </div>
                  <div className={styles.answerRow}>
                    <span className={styles.answerLabel}>{t('mistakes.correctAnswer')}</span>
                    <span className={`${styles.answerValue} ${styles.correctAnswer}`}>{item.correct_answer}</span>
                  </div>
                </div>
                {item.explanation && (
                  <div className={styles.explanation}>
                    <span className={styles.answerLabel}>{t('mistakes.explanation')}</span>
                    <p className={styles.explanationText}>{item.explanation}</p>
                  </div>
                )}
                <div className={styles.cardFooter}>
                  {item.source_path.startsWith('/') && (
                    <Link className={styles.practiceLink} to={item.source_path}>{t('mistakes.practice')}</Link>
                  )}
                  <Button
                    variant={item.status === 'mastered' ? 'secondary' : 'primary'}
                    size="sm"
                    loading={pendingId === item.id}
                    disabled={pendingId !== null}
                    onClick={() => void handleStatusChange(item)}
                  >
                    {item.status === 'mastered' ? t('mistakes.reAddToReview') : t('mistakes.markMastered')}
                  </Button>
                </div>
              </article>
            ))}
          </div>


        </>
      )}
          {!loading && !loadFailed && (page > 1 || totalPages > 1) && (
            <nav className={styles.pagination} aria-label={t('mistakes.pagination')}>
              <Button variant="secondary" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>
                {t('mistakes.previous')}
              </Button>
              <span className={styles.pageInfo}>{t('mistakes.page', { page, totalPages })}</span>
              <Button variant="secondary" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>
                {t('mistakes.next')}
              </Button>
            </nav>
          )}
    </div>
  )
}
