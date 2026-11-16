import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { useAuth } from '@/contexts/AuthContext'
import { Badge } from '@/components/ui/Badge'
import { EmptyState } from '@/components/ui/EmptyState'
import { Spinner } from '@/components/ui/Spinner'
import {
  CATALOG_LEVELS, CATALOG_SIZE, catalogParams, getCatalogFilters,
  type CatalogFilters, type LessonCatalogPage,
} from './lessonCatalog'
import styles from './LessonPage.module.css'

type LoadResult =
  | { key: string; data: LessonCatalogPage; failed?: never }
  | { key: string; data?: never; failed: true }

export function LessonListPage() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const [searchParams, setSearchParams] = useSearchParams()
  const filters = getCatalogFilters(searchParams)
  const { level, q, page } = filters
  const query = catalogParams(filters).toString()
  const request = catalogParams(filters)
  request.set('size', String(CATALOG_SIZE))
  const requestPath = `/api/v1/lessons/catalog?${request}`
  const [retry, setRetry] = useState(0)
  const requestKey = `${requestPath}:${retry}`
  const [result, setResult] = useState<LoadResult>()
  const current = result?.key === requestKey ? result : undefined
  const data = current?.data
  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.size)) : 1
  const pageOutOfRange = !!data && page > totalPages
  const summary = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const controller = new AbortController()
    apiFetch<LessonCatalogPage>('GET', requestPath, undefined, controller.signal)
      .then((data) => {
        if (!controller.signal.aborted) setResult({ key: requestKey, data })
      })
      .catch(() => {
        if (!controller.signal.aborted) setResult({ key: requestKey, failed: true })
      })
    return () => controller.abort()
  }, [requestKey, requestPath])

  useEffect(() => {
    if (pageOutOfRange) {
      setSearchParams(catalogParams({ level, q, page: totalPages }), { replace: true })
    }
  }, [pageOutOfRange, level, q, totalPages, setSearchParams])

  function update(next: CatalogFilters) {
    setSearchParams(catalogParams(next))
  }

  function search(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const keyword = String(new FormData(event.currentTarget).get('q') ?? '').trim()
    if (keyword !== q) update({ level, q: keyword, page: 1 })
  }

  function changePage(nextPage: number) {
    update({ level, q, page: nextPage })
    summary.current?.focus({ preventScroll: true })
    summary.current?.scrollIntoView({ block: 'start' })
  }

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>{t('lesson.title')}</h1>

      <div className={styles.catalogControls}>
        <div className={styles.levelFilters} role="group" aria-label={t('lesson.catalog.level')}>
          {CATALOG_LEVELS.map((option) => (
            <button
              key={option}
              type="button"
              aria-pressed={level === option}
              onClick={() => { if (level !== option) update({ level: option, q, page: 1 }) }}
            >
              {option || t('lesson.catalog.all')}
            </button>
          ))}
        </div>
        <form key={q} className={styles.catalogSearch} role="search" aria-label={t('lesson.catalog.search')} onSubmit={search}>
          <input name="q" type="search" aria-label={t('lesson.catalog.placeholder')} placeholder={t('lesson.catalog.placeholder')} defaultValue={q} />
          <button type="submit">{t('lesson.catalog.search')}</button>
          {q && <button type="button" onClick={() => update({ level, q: '', page: 1 })}>{t('lesson.catalog.clear')}</button>}
        </form>
      </div>

      <div className={styles.catalogSummary} ref={summary} tabIndex={-1} aria-live="polite">
        {data && !pageOutOfRange && t('lesson.catalog.total', { count: data.total })}
      </div>
      {!current || pageOutOfRange ? (
        <div className={styles.catalogLoading}><Spinner /></div>
      ) : current.failed || !data ? (
        <div className={styles.catalogError} role="alert">
          <p>{t('lesson.catalog.error')}</p>
          <button type="button" onClick={() => setRetry((value) => value + 1)}>{t('lesson.annotations.retry')}</button>
        </div>
      ) : data.items.length === 0 ? (
        <EmptyState title={t(q ? 'lesson.catalog.noMatches' : 'lesson.catalog.empty')} />
      ) : (
        <>
          <div className={styles.list}>
            {data.items.map((lesson) => (
              <Link
                key={lesson.id}
                className={styles.lessonItem}
                to={`/lesson/${lesson.id}`}
                state={{ lessonListSearch: query }}
              >
                <div className={styles.lessonLeft}>
                  <div className={styles.lessonTitle}>
                    {user && localStorage.getItem(`lesson_read_${user.id}_${lesson.id}`) === '1' && (
                      <span className={styles.readMark} title={t('lesson.catalog.read')} aria-label={t('lesson.catalog.read')}>✓</span>
                    )}
                    <span lang="ja">{lesson.title}</span>
                  </div>
                  <div className={styles.lessonMeta}>
                    <span className={styles.charCount}>{t('lesson.chars', { count: lesson.char_count })}</span>
                    {lesson.tags?.length > 0 && (
                      <div className={styles.tags}>
                        {lesson.tags.map((tag) => <span key={tag} className={styles.tag}>{tag}</span>)}
                      </div>
                    )}
                  </div>
                </div>
                <div className={styles.lessonRight}>
                  <Badge level={lesson.jlpt_level} size="sm" />
                  <span className={styles.arrow} aria-hidden="true">›</span>
                </div>
              </Link>
            ))}
          </div>
          {totalPages > 1 && (
            <nav className={styles.catalogPagination} aria-label={t('lesson.catalog.pagination')}>
              <button type="button" onClick={() => changePage(1)} disabled={page <= 1}>{t('lesson.catalog.first')}</button>
              <button type="button" onClick={() => changePage(page - 1)} disabled={page <= 1}>{t('lesson.catalog.previous')}</button>
              <span>{t('lesson.catalog.page', { page, totalPages })}</span>
              <button type="button" onClick={() => changePage(page + 1)} disabled={page >= totalPages}>{t('lesson.catalog.next')}</button>
              <button type="button" onClick={() => changePage(totalPages)} disabled={page >= totalPages}>{t('lesson.catalog.last')}</button>
            </nav>
          )}
        </>
      )}
    </div>
  )
}
