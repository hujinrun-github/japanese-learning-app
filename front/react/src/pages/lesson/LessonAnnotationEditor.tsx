import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  searchAnnotationReferences,
  type AnnotationReference,
  type LessonAnnotation,
  type ReferenceKind,
} from './lessonAnnotations'
import styles from './LessonAnnotation.module.css'

interface EditorProps {
  annotation: LessonAnnotation
  saved: boolean
  onSave: (annotation: LessonAnnotation) => Promise<void>
  onRemove: () => Promise<void>
  onClose: () => void
}

function referenceKey(link: AnnotationReference) {
  return `${link.kind}:${link.id}`
}

export function LessonAnnotationEditor({ annotation, saved, onSave, onRemove, onClose }: EditorProps) {
  const { t } = useTranslation()
  const dialog = useRef<HTMLDialogElement>(null)
  const currentSearch = useRef<AbortController | null>(null)
  const [comment, setComment] = useState(annotation.comment)
  const [links, setLinks] = useState(annotation.links)
  const [kind, setKind] = useState<ReferenceKind>('word')
  const [query, setQuery] = useState(Array.from(annotation.quote).length <= 24 ? annotation.quote : '')
  const [results, setResults] = useState<AnnotationReference[] | null>(null)
  const [searching, setSearching] = useState(false)
  const [searchFailed, setSearchFailed] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    dialog.current?.showModal()
    return () => currentSearch.current?.abort()
  }, [])

  async function search(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!query.trim() || searching) return
    const controller = new AbortController()
    currentSearch.current = controller
    setSearching(true)
    setSearchFailed(false)
    setResults(null)
    try {
      const matches = await searchAnnotationReferences(kind, query.trim(), controller.signal)
      if (!controller.signal.aborted) setResults(matches ?? [])
    } catch {
      if (!controller.signal.aborted) setSearchFailed(true)
    } finally {
      if (!controller.signal.aborted) setSearching(false)
    }
  }

  function resetSearch() {
    currentSearch.current?.abort()
    setSearching(false)
    setSearchFailed(false)
    setResults(null)
  }

  async function commit(remove: boolean) {
    if (pending) return
    setPending(true)
    setError('')
    try {
      if (remove) {
        await onRemove()
      } else {
        await onSave({ ...annotation, comment: comment.trim(), links })
      }
      onClose()
    } catch {
      setError(t(remove ? 'lesson.annotations.removeFailed' : 'lesson.annotations.saveFailed'))
      setPending(false)
    }
  }

  return (
    <dialog
      ref={dialog}
      className={styles.dialog}
      aria-labelledby="lesson-annotation-heading"
      onCancel={(event) => { event.preventDefault(); if (!pending) onClose() }}
    >
      <h2 id="lesson-annotation-heading">{t(saved ? 'lesson.annotations.edit' : 'lesson.annotations.create')}</h2>
      <blockquote className={styles.quote} lang="ja">{annotation.quote}</blockquote>

      <label className={styles.field} htmlFor="lesson-annotation-comment">{t('lesson.annotations.comment')}</label>
      <textarea id="lesson-annotation-comment" rows={3} maxLength={2000} value={comment} onChange={(event) => setComment(event.target.value)} />

      <h3 className={styles.field}>{t('lesson.annotations.references')}</h3>
      <form className={styles.search} onSubmit={search}>
        <select aria-label={t('lesson.annotations.referenceKind')} value={kind} onChange={(event) => {
          setKind(event.target.value as ReferenceKind)
          resetSearch()
        }}>
          <option value="word">{t('lesson.annotations.word')}</option>
          <option value="grammar">{t('lesson.annotations.grammar')}</option>
        </select>
        <input aria-label={t('lesson.annotations.query')} type="search" value={query} maxLength={80} onChange={(event) => {
          setQuery(event.target.value)
          resetSearch()
        }} />
        <button type="submit" disabled={searching || !query.trim()}>{t('lesson.annotations.search')}</button>
      </form>
      {searching && <p role="status">{t('lesson.annotations.searching')}</p>}
      {searchFailed && <p role="alert">{t('lesson.annotations.searchFailed')}</p>}
      {results?.length === 0 && <p role="status">{t('lesson.annotations.noMatches')}</p>}
      {!!results?.length && (
        <ul className={styles.results}>
          {results.map((item) => {
            const linked = links.some((link) => referenceKey(link) === referenceKey(item))
            return (
              <li key={item.id}>
                <div><strong lang="ja">{item.title}{item.reading && ` · ${item.reading}`}</strong><small>{item.meaning}</small></div>
                <button type="button" disabled={linked} onClick={() => setLinks((items) => [...items, item])}>
                  {t(linked ? 'lesson.annotations.linked' : 'lesson.annotations.link')}
                </button>
              </li>
            )
          })}
        </ul>
      )}

      {links.length > 0 && (
        <ul className={styles.links}>
          {links.map((link) => (
            <li key={referenceKey(link)}>
              <div>
                <small>{t(`lesson.annotations.${link.kind}`)}</small>
                {link.kind === 'grammar' ? (
                  <Link to={`/grammar/${link.id}`} target="_blank" rel="noopener" lang="ja">{link.title}</Link>
                ) : (
                  <strong lang="ja">{link.title}{link.reading && ` · ${link.reading}`}</strong>
                )}
                <small>{link.meaning}</small>
              </div>
              <button type="button" onClick={() => setLinks((items) => items.filter((item) => referenceKey(item) !== referenceKey(link)))}>
                {t('lesson.annotations.unlink')}
              </button>
            </li>
          ))}
        </ul>
      )}

      {error && <p className={styles.error} role="alert">{error}</p>}
      <div className={styles.actions}>
        {saved && <button disabled={pending} type="button" className={styles.remove} onClick={() => void commit(true)}>{t('lesson.annotations.remove')}</button>}
        <button disabled={pending} type="button" onClick={onClose}>{t('notes.cancel')}</button>
        <button disabled={pending} type="button" className={styles.primary} onClick={() => void commit(false)}>{t('lesson.annotations.save')}</button>
      </div>
    </dialog>
  )
}
