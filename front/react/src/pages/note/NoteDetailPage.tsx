import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'


import { Spinner } from '@/components/ui/Spinner'
import type { Note, NoteDetail, NoteLink, LinkRelation, PaginatedNotes } from '@/types/api'
import styles from './NoteDetailPage.module.css'

const TYPE_COLORS: Record<string, string> = {
  word: 'var(--color-primary)',
  grammar: 'var(--color-warning)',
  sentence: 'var(--color-success)',
}

const RELATIONS: LinkRelation[] = ['related', 'uses_word', 'uses_grammar', 'context']

export function NoteDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { t } = useTranslation()

  const [note, setNote] = useState<NoteDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  // Edit state
  const [editing, setEditing] = useState(false)
  const [editTitle, setEditTitle] = useState('')
  const [editContent, setEditContent] = useState('')
  const [editSourceText, setEditSourceText] = useState('')
  const [editTags, setEditTags] = useState<string[]>([])
  const [editTagInput, setEditTagInput] = useState('')
  const [saving, setSaving] = useState(false)

  // Link modal state
  const [linkModalOpen, setLinkModalOpen] = useState(false)
  const [linkSearch, setLinkSearch] = useState('')
  const [linkResults, setLinkResults] = useState<Note[]>([])
  const [linkSearching, setLinkSearching] = useState(false)
  const [linkRelation, setLinkRelation] = useState<LinkRelation>('related')

  // Delete state
  const [deleting, setDeleting] = useState(false)

  useEffect(() => {
    loadNote()
  }, [id])

  async function loadNote() {
    setLoading(true)
    setError('')
    try {
      const n = await apiFetch<NoteDetail>('GET', `/api/v1/notes/${id}`)
      setNote(n ?? null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }

  function startEditing() {
    if (!note) return
    setEditTitle(note.title)
    setEditContent(note.content ?? '')
    setEditSourceText(note.source_text ?? '')
    setEditTags([...(note.tags ?? [])])
    setEditTagInput('')
    setEditing(true)
  }

  function cancelEditing() {
    setEditing(false)
  }

  function handleEditTagKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && editTagInput.trim()) {
      e.preventDefault()
      const tag = editTagInput.trim()
      if (!editTags.includes(tag)) {
        setEditTags([...editTags, tag])
      }
      setEditTagInput('')
    }
  }

  function removeEditTag(tag: string) {
    setEditTags(editTags.filter((t) => t !== tag))
  }

  async function handleSave() {
    if (!editTitle.trim()) return
    setSaving(true)
    try {
      await apiFetch('PUT', `/api/v1/notes/${id}`, {
        title: editTitle.trim(),
        content: editContent,
        source_text: editSourceText,
        tags: editTags,
      })
      setEditing(false)
      await loadNote()
    } catch (err) {
      // keep editing mode on error
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete() {
    if (!window.confirm(t('notes.deleteConfirm'))) return
    setDeleting(true)
    try {
      await apiFetch('DELETE', `/api/v1/notes/${id}`)
      navigate('/notes')
    } catch {
      // ignore
    } finally {
      setDeleting(false)
    }
  }

  async function handleSearchLinks() {
    if (!linkSearch.trim()) return
    setLinkSearching(true)
    try {
      const res = await apiFetch<PaginatedNotes>('GET', `/api/v1/notes/search?q=${encodeURIComponent(linkSearch.trim())}`)
      const items = (res?.items ?? []).filter((n: Note) => n.id !== Number(id))
      setLinkResults(items)
    } catch {
      setLinkResults([])
    } finally {
      setLinkSearching(false)
    }
  }

  async function handleAddLink(targetNoteId: number) {
    try {
      await apiFetch<NoteLink>('POST', `/api/v1/notes/${id}/links`, {
        target_note_id: targetNoteId,
        relation: linkRelation,
      })
      setLinkModalOpen(false)
      setLinkSearch('')
      setLinkResults([])
      await loadNote()
    } catch {
      // ignore
    }
  }

  async function handleRemoveLink(linkId: number) {
    try {
      await apiFetch('DELETE', `/api/v1/notes/${id}/links/${linkId}`)
      await loadNote()
    } catch {
      // ignore
    }
  }

  if (loading) {
    return (
      <div className={styles.page} style={{ display: 'flex', justifyContent: 'center', paddingTop: '80px' }}>
        <Spinner size="lg" />
      </div>
    )
  }

  if (error || !note) {
    return (
      <div className={styles.page}>
        <p style={{ color: 'var(--color-error)' }}>{error || 'Note not found'}</p>
        <button className={styles.backLink} onClick={() => navigate('/notes')}>&lt; {t('notes.cancel')}</button>
      </div>
    )
  }

  // Display mode vs edit mode
  return (
    <div className={styles.page}>
      {/* Header */}
      <div className={styles.header}>
        <button className={styles.backLink} onClick={() => navigate('/notes')}>&lt; {t('notes.cancel')}</button>
        <div className={styles.headerActions}>
          {!editing ? (
            <>
              <button className={styles.actionBtn} onClick={startEditing}>{t('notes.edit')}</button>
              <button className={styles.actionBtn} onClick={handleDelete} disabled={deleting}>
                {deleting ? <Spinner size="sm" /> : t('notes.delete')}
              </button>
            </>
          ) : (
            <>
              <button className={styles.actionBtn} onClick={cancelEditing}>{t('notes.cancel')}</button>
              <button className={`${styles.actionBtn} ${styles.saveBtn}`} onClick={handleSave} disabled={saving}>
                {saving ? <Spinner size="sm" /> : t('notes.save')}
              </button>
            </>
          )}
        </div>
      </div>

      {editing ? (
        /* ============== EDIT MODE ============== */
        <div className={styles.editForm}>
          {/* Title */}
          <div className={styles.field}>
            <label className={styles.label}>Title</label>
            <input
              className={styles.input}
              type="text"
              value={editTitle}
              onChange={(e) => setEditTitle(e.target.value)}
            />
          </div>

          {/* Content */}
          <div className={styles.field}>
            <label className={styles.label}>{t('notes.content')}</label>
            <textarea
              className={styles.textarea}
              value={editContent}
              onChange={(e) => setEditContent(e.target.value)}
              rows={5}
            />
          </div>

          {/* Source text */}
          <div className={styles.field}>
            <label className={styles.label}>{t('notes.sourceText')}</label>
            <input
              className={styles.input}
              type="text"
              value={editSourceText}
              onChange={(e) => setEditSourceText(e.target.value)}
            />
          </div>

          {/* Tags */}
          <div className={styles.field}>
            <label className={styles.label}>{t('notes.tags')}</label>
            <input
              className={styles.input}
              type="text"
              value={editTagInput}
              onChange={(e) => setEditTagInput(e.target.value)}
              onKeyDown={handleEditTagKeyDown}
              placeholder={t('notes.tagPlaceholder')}
            />
            {editTags.length > 0 && (
              <div className={styles.tagRow}>
                {editTags.map((tag) => (
                  <span key={tag} className={styles.tag}>
                    {tag}
                    <button type="button" className={styles.tagRemove} onClick={() => removeEditTag(tag)}>&times;</button>
                  </span>
                ))}
              </div>
            )}
          </div>
        </div>
      ) : (
        /* ============== DISPLAY MODE ============== */
        <>
          {/* Type + Title */}
          <div className={styles.typeRow}>
            <span className={styles.typeBadge} style={{ background: TYPE_COLORS[note.type] ?? 'var(--color-text-disabled)' }}>
              {t(`notes.filter${note.type.charAt(0).toUpperCase() + note.type.slice(1)}`)}
            </span>
            <h1 className={styles.title}>{note.title}</h1>
          </div>

          {/* Tags */}
          {(note.tags ?? []).length > 0 && (
            <div className={styles.tagRow}>
              {(note.tags ?? []).map((tag) => (
                <span key={tag} className={styles.tag}>{tag}</span>
              ))}
            </div>
          )}

          {/* Content */}
          {note.content && (
            <section className={styles.section}>
              <h2 className={styles.sectionTitle}>{t('notes.content')}</h2>
              <p className={styles.content}>{note.content}</p>
            </section>
          )}

          {/* Source text */}
          {note.source_text && (
            <section className={styles.section}>
              <h2 className={styles.sectionTitle}>{t('notes.sourceText')}</h2>
              <p className={styles.sourceText}>{note.source_text}</p>
            </section>
          )}

          {/* Reference */}
          {note.reference_id && note.reference_type ? (
            <section className={styles.section}>
              <h2 className={styles.sectionTitle}>{t('notes.reference')}</h2>
              <p className={styles.reference}>
                {note.reference_type}: {note.reference_id}
              </p>
            </section>
          ) : null}

          {/* Linked notes */}
          <section className={styles.section}>
            <div className={styles.sectionHeader}>
              <h2 className={styles.sectionTitle}>{t('notes.links')}</h2>
              <button className={styles.addLinkBtn} onClick={() => setLinkModalOpen(true)}>
                + {t('notes.addLink')}
              </button>
            </div>
            {(note.links ?? []).length === 0 ? (
              <p className={styles.emptyText}>{t('notes.noLinks')}</p>
            ) : (
              <div className={styles.linkList}>
                {(note.links ?? []).map((link) => (
                  <div key={link.id} className={styles.linkItem}>
                    <div
                      className={styles.linkInfo}
                      onClick={() => navigate(`/notes/${link.target_note_id}`)}
                      role="button"
                      tabIndex={0}
                      onKeyDown={(e) => { if (e.key === 'Enter') navigate(`/notes/${link.target_note_id}`) }}
                    >
                      <span className={styles.linkTitle}>
                        {link.target_note?.title ?? `Note #${link.target_note_id}`}
                      </span>
                      <span className={styles.linkRelation}>
                        {t(`notes.relation.${link.relation}`)}
                      </span>
                    </div>
                    <button
                      className={styles.linkRemoveBtn}
                      onClick={() => handleRemoveLink(link.id)}
                      aria-label="Remove link"
                    >
                      &times;
                    </button>
                  </div>
                ))}
              </div>
            )}
          </section>

          {/* Backlinks */}
          <section className={styles.section}>
            <h2 className={styles.sectionTitle}>{t('notes.backlinks')}</h2>
            {(note.backlinks ?? []).length === 0 ? (
              <p className={styles.emptyText}>{t('notes.noBacklinks')}</p>
            ) : (
              <div className={styles.linkList}>
                {(note.backlinks ?? []).map((link) => (
                  <div key={link.id} className={styles.linkItem}>
                    <div
                      className={styles.linkInfo}
                      onClick={() => navigate(`/notes/${link.note_id}`)}
                      role="button"
                      tabIndex={0}
                      onKeyDown={(e) => { if (e.key === 'Enter') navigate(`/notes/${link.note_id}`) }}
                    >
                      <span className={styles.linkTitle}>
                        {link.target_note?.title ?? `Note #${link.note_id}`}
                      </span>
                      <span className={styles.linkRelation}>
                        {t(`notes.relation.${link.relation}`)}
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </section>

          {/* Timestamps */}
          <p className={styles.timestamps}>
            {new Date(note.created_at).toLocaleDateString()} · {new Date(note.updated_at).toLocaleDateString()}
          </p>
        </>
      )}

      {/* ============== LINK SEARCH MODAL ============== */}
      {linkModalOpen && (
        <div className={styles.modalOverlay} onClick={() => setLinkModalOpen(false)}>
          <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
            <h3 className={styles.modalTitle}>{t('notes.searchLink')}</h3>

            {/* Search input */}
            <div className={styles.searchRow}>
              <input
                className={styles.input}
                type="text"
                value={linkSearch}
                onChange={(e) => setLinkSearch(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') handleSearchLinks() }}
                placeholder={t('notes.searchLink')}
              />
              <button className={styles.searchBtn} onClick={handleSearchLinks} disabled={linkSearching}>
                {linkSearching ? <Spinner size="sm" /> : '🔍'}
              </button>
            </div>

            {/* Relation selector */}
            <div className={styles.field}>
              <label className={styles.label}>Relation</label>
              <div className={styles.relationRow}>
                {RELATIONS.map((r) => (
                  <button
                    key={r}
                    type="button"
                    className={`${styles.relationBtn} ${linkRelation === r ? styles.relationActive : ''}`}
                    onClick={() => setLinkRelation(r)}
                  >
                    {t(`notes.relation.${r}`)}
                  </button>
                ))}
              </div>
            </div>

            {/* Results */}
            <div className={styles.searchResults}>
              {linkResults.length === 0 && linkSearch && !linkSearching && (
                <p className={styles.emptyText}>{t('notes.noResults')}</p>
              )}
              {linkResults.map((n) => (
                <div
                  key={n.id}
                  className={styles.searchItem}
                  onClick={() => handleAddLink(n.id)}
                  role="button"
                  tabIndex={0}
                  onKeyDown={(e) => { if (e.key === 'Enter') handleAddLink(n.id) }}
                >
                  <span className={styles.typeDot} style={{ background: TYPE_COLORS[n.type] ?? 'var(--color-text-disabled)' }} />
                  <span className={styles.searchItemTitle}>{n.title}</span>
                </div>
              ))}
            </div>

            <button className={styles.modalCloseBtn} onClick={() => setLinkModalOpen(false)}>
              {t('notes.cancel')}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
