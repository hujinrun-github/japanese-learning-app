import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { APIError, apiFetch } from '@/api/client'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { Spinner } from '@/components/ui/Spinner'
import dialogStyles from '@/components/ui/ErrataButton.module.css'
import {
  allowsLabelEdit,
  buildEditPayload,
  canDelete,
  canEdit,
  errataCategories,
  type ErrataReport,
} from './errataEditRules'
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

type ReportPage = { items: ErrataReport[]; has_more: boolean }
type CardMode = 'idle' | 'edit' | 'delete'

export function ErrataHistoryPage() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [attempt, setAttempt] = useState(0)
  const [data, setData] = useState<ReportPage | null>(null)
  const [failed, setFailed] = useState(false)

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

  function handleChanged(removed: boolean) {
    if (removed && data?.items.length === 1 && page > 1) {
      setPage(value => value - 1)
    } else {
      setAttempt(value => value + 1)
    }
  }

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
                <ErrataReportCard key={report.id} report={report} onChanged={handleChanged} />
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

export function ErrataReportCard({ report, onChanged }: { report: ErrataReport; onChanged: (removed: boolean) => void }) {
  const { t, i18n } = useTranslation()
  const [mode, setMode] = useState<CardMode>('idle')
  const dateFormat = new Intl.DateTimeFormat(i18n.resolvedLanguage, { dateStyle: 'medium', timeStyle: 'short' })

  return (
    <li className={styles.card}>
      <div className={styles.cardHeader}>
        <p>{t(moduleLabels[report.module])} · {t(`errata.categories.${report.category}`)}</p>
        <span className={`${styles.status} ${styles[report.status]}`}>{t(`errata.history.${report.status}`)}</span>
      </div>
      <h2>{report.item_label}</h2>
      <p className={styles.description}>{report.description}</p>
      <footer className={styles.cardFooter}>
        <div className={styles.meta}>
          <span>{t('errata.history.reportNumber', { id: report.id })}</span>
          <time dateTime={report.created_at}>{dateFormat.format(new Date(report.created_at))}</time>
        </div>
        {canEdit(report) ? (
          <div className={styles.actions}>
            <Button variant="ghost" size="sm" onClick={() => setMode('edit')}>✏️ {t('errata.history.edit')}</Button>
            {canDelete(report) && (
              <Button variant="ghost" size="sm" onClick={() => setMode('delete')}>🗑️ {t('errata.history.delete')}</Button>
            )}
          </div>
        ) : (
          <p className={styles.locked}>{t('errata.history.lockedHint')}</p>
        )}
      </footer>

      {mode === 'edit' && (
        <EditDialog
          report={report}
          onClose={() => setMode('idle')}
          onSaved={() => { setMode('idle'); onChanged(false) }}
        />
      )}
      {mode === 'delete' && (
        <DeleteDialog
          report={report}
          onClose={() => setMode('idle')}
          onDeleted={() => { setMode('idle'); onChanged(true) }}
        />
      )}
    </li>
  )
}

function EditDialog({ report, onClose, onSaved }: {
  report: ErrataReport
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const titleId = useId()
  const dialog = useRef<HTMLDialogElement>(null)
  const [category, setCategory] = useState(report.category)
  const [itemLabel, setItemLabel] = useState(report.item_label)
  const [description, setDescription] = useState(report.description)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => { dialog.current?.showModal() }, [])

  const payload = buildEditPayload({ category, itemLabel, description }, report)
  const valid = description.trim() !== '' && (!allowsLabelEdit(category) || itemLabel.trim() !== '')
  const canSubmit = Object.keys(payload).length > 0 && valid && !busy

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!canSubmit) return
    setBusy(true)
    setError('')
    try {
      await apiFetch('PATCH', `/api/v1/errata/${report.id}`, payload)
      onSaved()
    } catch (err) {
      const code = err instanceof APIError ? err.code : ''
      setError(code === 'ERR_ERRATA_RESOLVED' ? t('errata.history.lockedError') : t('errata.history.editError'))
    } finally {
      setBusy(false)
    }
  }

  function changeCategory(value: string) {
    setCategory(value)
    if (!allowsLabelEdit(value)) setItemLabel(report.item_label)
  }

  return (
    <dialog
      ref={dialog}
      className={dialogStyles.dialog}
      aria-labelledby={titleId}
      onClose={onClose}
      onCancel={(event) => { if (busy) event.preventDefault() }}
    >
      <h2 id={titleId} className={dialogStyles.title}>{t('errata.history.editTitle')}</h2>
      <p>{t('errata.history.editHint')}</p>
      <p className={dialogStyles.item}>{t('errata.history.reportNumber', { id: report.id })} · {report.item_key}</p>
      <form className={dialogStyles.form} onSubmit={save}>
        <label>
          {t('errata.category')}
          <select value={category} onChange={(event) => changeCategory(event.target.value)} disabled={busy || report.category === 'audio'}>
            {errataCategories.map(option => (
              <option key={option} value={option}>{t(`errata.categories.${option}`)}</option>
            ))}
          </select>
        </label>
        {allowsLabelEdit(category) && (
          <label>
            {t('errata.history.itemLabel')}
            <textarea rows={2} maxLength={300} required value={itemLabel}
              onChange={(event) => setItemLabel(event.target.value)} disabled={busy} />
          </label>
        )}
        <label>
          {t('errata.description')}
          <textarea rows={4} maxLength={1000} required value={description}
            onChange={(event) => setDescription(event.target.value)} disabled={busy} />
        </label>
        {error && <p role="alert" className={dialogStyles.error}>{error}</p>}
        <div className={dialogStyles.actions}>
          <Button type="button" variant="secondary" disabled={busy} onClick={() => dialog.current?.close()}>
            {t('errata.cancel')}
          </Button>
          <Button type="submit" loading={busy} disabled={!canSubmit}>{t('errata.history.save')}</Button>
        </div>
      </form>
    </dialog>
  )
}

function DeleteDialog({ report, onClose, onDeleted }: {
  report: ErrataReport
  onClose: () => void
  onDeleted: () => void
}) {
  const { t } = useTranslation()
  const titleId = useId()
  const dialog = useRef<HTMLDialogElement>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => { dialog.current?.showModal() }, [])

  async function remove() {
    if (busy) return
    setBusy(true)
    setError('')
    try {
      await apiFetch('DELETE', `/api/v1/errata/${report.id}`)
      onDeleted()
    } catch (err) {
      const code = err instanceof APIError ? err.code : ''
      setError(code === 'ERR_ERRATA_DELETE_RESTRICTED' ? t('errata.history.restrictedError')
        : code === 'ERR_ERRATA_RESOLVED' ? t('errata.history.lockedError')
          : t('errata.history.deleteError'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <dialog
      ref={dialog}
      className={dialogStyles.dialog}
      aria-labelledby={titleId}
      onClose={onClose}
      onCancel={(event) => { if (busy) event.preventDefault() }}
    >
      <h2 id={titleId} className={dialogStyles.title}>{t('errata.history.deleteTitle')}</h2>
      <p className={dialogStyles.item}>{report.item_label}</p>
      <p className={styles.notice}>{t('errata.history.deleteHint')}</p>
      {error && <p role="alert" className={dialogStyles.error}>{error}</p>}
      <div className={dialogStyles.actions}>
        <Button type="button" variant="secondary" disabled={busy} onClick={() => dialog.current?.close()}>
          {t('errata.cancel')}
        </Button>
        <Button type="button" variant="danger" loading={busy} onClick={() => void remove()}>
          {t('errata.history.confirmDelete')}
        </Button>
      </div>
    </dialog>
  )
}
