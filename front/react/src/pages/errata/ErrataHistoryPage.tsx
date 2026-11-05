import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { Spinner } from '@/components/ui/Spinner'
import styles from './ErrataHistoryPage.module.css'

const moduleLabels = {
  word: 'nav.words',
  grammar: 'nav.grammar',
  lesson: 'nav.lesson',
  speaking: 'nav.speaking',
  shadowing: 'errata.history.shadowing',
  writing: 'nav.writing',
  translation: 'nav.translation',
}

type Report = {
  id: number
  module: keyof typeof moduleLabels
  item_label: string
  category: string
  description: string
  status: 'pending' | 'resolved'
  created_at: string
}

type ReportPage = { items: Report[]; has_more: boolean }

export function ErrataHistoryPage() {
  const { t, i18n } = useTranslation()
  const [page, setPage] = useState(1)
  const [attempt, setAttempt] = useState(0)
  const [data, setData] = useState<ReportPage | null>(null)
  const [failed, setFailed] = useState(false)
  const dateFormat = new Intl.DateTimeFormat(i18n.resolvedLanguage, { dateStyle: 'medium', timeStyle: 'short' })

  useEffect(() => {
    const controller = new AbortController()
    setData(null)
    setFailed(false)

    async function loadReports() {
      try {
        const result = await apiFetch<ReportPage>('GET', `/api/v1/errata?page=${page}`, undefined, controller.signal)
        if (!result) throw new Error('Missing errata history data')
        if (!controller.signal.aborted) setData(result)
      } catch {
        if (!controller.signal.aborted) setFailed(true)
      }
    }

    void loadReports()
    return () => controller.abort()
  }, [page, attempt])

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <h1>{t('errata.history.title')}</h1>
        <p>{t('errata.history.intro')}</p>
      </header>

      {failed ? (
        <div className={styles.message}>
          <p role="alert">{t('errata.history.error')}</p>
          <Button variant="secondary" onClick={() => setAttempt(value => value + 1)}>{t('errata.history.retry')}</Button>
        </div>
      ) : data === null ? (
        <div className={styles.message}><Spinner size="lg" /></div>
      ) : (
        <>
          {data.items.length === 0 ? (
            <EmptyState icon="📝" title={t('errata.history.empty')} description={t('errata.history.emptyDesc')} />
          ) : (
            <ul className={styles.list}>
              {data.items.map(report => (
                <li className={styles.card} key={report.id}>
                  <div className={styles.cardHeader}>
                    <p>{t(moduleLabels[report.module])} · {t(`errata.categories.${report.category}`)}</p>
                    <span className={`${styles.status} ${styles[report.status]}`}>{t(`errata.history.${report.status}`)}</span>
                  </div>
                  <h2>{report.item_label}</h2>
                  <p className={styles.description}>{report.description}</p>
                  <footer className={styles.meta}>
                    <span>{t('errata.history.reportNumber', { id: report.id })}</span>
                    <time dateTime={report.created_at}>{dateFormat.format(new Date(report.created_at))}</time>
                  </footer>
                </li>
              ))}
            </ul>
          )}

          {(page > 1 || data.has_more) && (
            <nav className={styles.pagination} aria-label={t('errata.history.pagination')}>
              <Button variant="secondary" disabled={page === 1} onClick={() => setPage(value => value - 1)}>
                {t('errata.history.previous')}
              </Button>
              <span>{t('errata.history.page', { page })}</span>
              <Button variant="secondary" disabled={!data.has_more} onClick={() => setPage(value => value + 1)}>
                {t('errata.history.next')}
              </Button>
            </nav>
          )}
        </>
      )}
    </div>
  )
}
