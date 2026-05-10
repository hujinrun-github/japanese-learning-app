import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Spinner } from '@/components/ui/Spinner'
import type { Note, NoteType } from '@/types/api'
import styles from './NoteEditPage.module.css'

const NOTE_TYPES: NoteType[] = ['word', 'grammar', 'sentence']

export function NoteEditPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [type, setType] = useState<NoteType>('word')
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [sourceText, setSourceText] = useState('')
  const [tags, setTags] = useState<string[]>([])
  const [tagInput, setTagInput] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  function handleTagKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && tagInput.trim()) {
      e.preventDefault()
      const tag = tagInput.trim()
      if (!tags.includes(tag)) {
        setTags([...tags, tag])
      }
      setTagInput('')
    }
  }

  function removeTag(tag: string) {
    setTags(tags.filter((t) => t !== tag))
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!title.trim()) return
    setSubmitting(true)
    setError('')
    try {
      const note = await apiFetch<Note>('POST', '/api/v1/notes', {
        type,
        title: title.trim(),
        content,
        source_text: sourceText,
        tags,
      })
      navigate(`/notes/${note.id}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <button className={styles.backBtn} onClick={() => navigate('/notes')}>
          &lt; {t('notes.cancel')}
        </button>
        <h1 className={styles.title}>{t('notes.create')}</h1>
      </div>

      <form className={styles.form} onSubmit={handleSubmit}>
        {/* Type selector */}
        <div className={styles.field}>
          <label className={styles.label}>{t('notes.type')}</label>
          <div className={styles.typeSelector}>
            {NOTE_TYPES.map((nt) => (
              <button
                key={nt}
                type="button"
                className={`${styles.typeBtn} ${type === nt ? styles.typeActive : ''}`}
                onClick={() => setType(nt)}
              >
                {t(`notes.filter${nt.charAt(0).toUpperCase() + nt.slice(1)}`)}
              </button>
            ))}
          </div>
        </div>

        {/* Title */}
        <div className={styles.field}>
          <label className={styles.label} htmlFor="note-title">{titleLabel[type]}</label>
          <input
            id="note-title"
            className={styles.input}
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder={titlePlaceholder[type]}
            required
          />
        </div>

        {/* Content */}
        <div className={styles.field}>
          <label className={styles.label} htmlFor="note-content">{t('notes.content')}</label>
          <textarea
            id="note-content"
            className={styles.textarea}
            value={content}
            onChange={(e) => setContent(e.target.value)}
            placeholder={t('notes.content')}
            rows={5}
          />
        </div>

        {/* Source text */}
        <div className={styles.field}>
          <label className={styles.label} htmlFor="note-source">{t('notes.sourceText')}</label>
          <input
            id="note-source"
            className={styles.input}
            type="text"
            value={sourceText}
            onChange={(e) => setSourceText(e.target.value)}
            placeholder={t('notes.sourceText')}
          />
        </div>

        {/* Tags */}
        <div className={styles.field}>
          <label className={styles.label}>{t('notes.tags')}</label>
          <input
            className={styles.input}
            type="text"
            value={tagInput}
            onChange={(e) => setTagInput(e.target.value)}
            onKeyDown={handleTagKeyDown}
            placeholder={t('notes.tagPlaceholder')}
          />
          {tags.length > 0 && (
            <div className={styles.tagRow}>
              {tags.map((tag) => (
                <span key={tag} className={styles.tag}>
                  {tag}
                  <button type="button" className={styles.tagRemove} onClick={() => removeTag(tag)}>&times;</button>
                </span>
              ))}
            </div>
          )}
        </div>

        {error && <p className={styles.error}>{error}</p>}

        <button
          type="submit"
          className={styles.submitBtn}
          disabled={submitting || !title.trim()}
        >
          {submitting ? <Spinner size="sm" /> : t('notes.save')}
        </button>
      </form>
    </div>
  )
}

const titleLabel: Record<string, string> = {
  word: '单词',
  grammar: '语法名',
  sentence: '句子',
}

const titlePlaceholder: Record<string, string> = {
  word: '例：雨',
  grammar: '例：～ている',
  sentence: '例：雨が降っている',
}
