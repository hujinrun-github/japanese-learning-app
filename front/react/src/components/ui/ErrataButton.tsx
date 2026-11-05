import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { apiFetch } from '@/api/client'
import { Button } from './Button'
import styles from './ErrataButton.module.css'

type ErrataItem = {
  module: 'word' | 'grammar' | 'lesson' | 'speaking' | 'shadowing' | 'writing' | 'translation'
  itemKey: string
  label: string
}

const categories = ['prompt', 'answer', 'explanation', 'audio', 'other'] as const

export function ErrataButton(item: ErrataItem) {
  const { t } = useTranslation()
  const [target, setTarget] = useState<ErrataItem | null>(null)

  return (
    <>
      <Button
        className={styles.trigger}
        type="button"
        variant="ghost"
        size="sm"
        aria-haspopup="dialog"
        onClick={() => setTarget({ ...item, label: Array.from(item.label.trim()).slice(0, 300).join('') || item.itemKey })}
      >
        {t('errata.open')}
      </Button>
      {target && <ErrataDialog item={target} onClose={() => setTarget(null)} />}
    </>
  )
}

function ErrataDialog({ item, onClose }: { item: ErrataItem; onClose: () => void }) {
  const { t } = useTranslation()
  const titleId = useId()
  const dialog = useRef<HTMLDialogElement>(null)
  const [category, setCategory] = useState<string>('prompt')
  const [description, setDescription] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [submitted, setSubmitted] = useState(false)
  const [error, setError] = useState(false)

  useEffect(() => {
    dialog.current?.showModal()
  }, [])

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const detail = description.trim()
    if (!detail || submitting) return
    setSubmitting(true)
    setError(false)
    try {
      await apiFetch('POST', '/api/v1/errata', {
        module: item.module,
        item_key: item.itemKey,
        item_label: item.label,
        category,
        description: detail,
      })
      setSubmitted(true)
    } catch {
      setError(true)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <dialog
      ref={dialog}
      className={styles.dialog}
      aria-labelledby={titleId}
      onClose={onClose}
      onCancel={(event) => { if (submitting) event.preventDefault() }}
    >
      <h2 id={titleId} className={styles.title}>{t('errata.title')}</h2>
      <p className={styles.item}>{item.label}</p>
      {submitted ? (
        <>
          <p role="status">{t('errata.success')}</p>
          <div className={styles.actions}>
            <Link className={styles.historyLink} to="/errata">{t('errata.history.view')}</Link>
            <Button type="button" onClick={() => dialog.current?.close()}>{t('errata.close')}</Button>
          </div>
        </>
      ) : (
        <form className={styles.form} onSubmit={submit}>
          <label>
            {t('errata.category')}
            <select value={category} onChange={(event) => setCategory(event.target.value)} disabled={submitting}>
              {categories.map((option) => (
                <option key={option} value={option}>{t(`errata.categories.${option}`)}</option>
              ))}
            </select>
          </label>
          <label>
            {t('errata.description')}
            <textarea
              rows={4}
              maxLength={1000}
              required
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              disabled={submitting}
            />
          </label>
          {error && <p className={styles.error} role="alert">{t('errata.error')}</p>}
          <div className={styles.actions}>
            <Button type="button" variant="secondary" disabled={submitting} onClick={() => dialog.current?.close()}>
              {t('errata.cancel')}
            </Button>
            <Button type="submit" disabled={!description.trim()} loading={submitting}>
              {t('errata.submit')}
            </Button>
          </div>
        </form>
      )}
    </dialog>
  )
}
