import { useState, useEffect, useCallback } from 'react'
import { adminFetch } from '@/api/client'
import Modal from '@/components/Modal/Modal'
import styles from './Translation.module.css'

interface TranslationSentence {
  id: number
  source_id: number
  direction: string
  source_text: string
  reference_translation: string
  position: number
}

const DIRECTIONS = ['', 'cn2jp', 'jp2cn']
const PAGE_SIZE = 20

const emptyForm = {
  source_id: 0,
  direction: 'cn2jp',
  source_text: '',
  reference_translation: '',
  position: 0,
}

export default function TranslationPage() {
  const [items, setItems] = useState<TranslationSentence[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [sourceId, setSourceId] = useState('')
  const [direction, setDirection] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<TranslationSentence | null>(null)
  const [form, setForm] = useState(emptyForm)

  const totalPages = Math.ceil(total / PAGE_SIZE)

  const fetchItems = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const params = new URLSearchParams({ page: String(page), size: String(PAGE_SIZE) })
      if (sourceId) params.set('source_id', sourceId)
      if (direction) params.set('direction', direction)
      const data = await adminFetch<{ items: TranslationSentence[]; total: number }>('GET', `/translation?${params}`)
      setItems(data.items)
      setTotal(data.total)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }, [page, sourceId, direction])

  useEffect(() => {
    fetchItems()
  }, [fetchItems])

  function handleSearch() {
    setPage(1)
  }

  function openCreate() {
    setEditing(null)
    setForm(emptyForm)
    setModalOpen(true)
  }

  function openEdit(s: TranslationSentence) {
    setEditing(s)
    setForm({
      source_id: s.source_id,
      direction: s.direction,
      source_text: s.source_text,
      reference_translation: s.reference_translation,
      position: s.position,
    })
    setModalOpen(true)
  }

  async function handleSave() {
    setError('')
    try {
      const body = {
        source_id: Number(form.source_id),
        direction: form.direction,
        source_text: form.source_text,
        reference_translation: form.reference_translation,
        position: Number(form.position),
      }
      if (editing) {
        await adminFetch('PUT', `/translation/${editing.id}`, body)
      } else {
        await adminFetch('POST', '/translation', body)
      }
      setModalOpen(false)
      fetchItems()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed')
    }
  }

  async function handleDelete(id: number) {
    if (!confirm('Delete this translation sentence?')) return
    setError('')
    try {
      await adminFetch('DELETE', `/translation/${id}`)
      fetchItems()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed')
    }
  }

  const columns = [
    { key: 'id', label: 'ID' },
    { key: 'source_id', label: 'Source ID' },
    { key: 'direction', label: 'Direction' },
    { key: 'source_text', label: 'Source Text' },
    { key: 'reference_translation', label: 'Reference' },
    { key: 'position', label: 'Position' },
  ]

  return (
    <div className={styles.page}>
      <h2 className={styles.title}>Translation</h2>

      <div className={styles.toolbar}>
        <select
          value={direction}
          onChange={(e) => {
            setDirection(e.target.value)
            setPage(1)
          }}
        >
          {DIRECTIONS.map((d) => (
            <option key={d} value={d}>
              {d || 'All Directions'}
            </option>
          ))}
        </select>
        <input
          placeholder="Source ID..."
          value={sourceId}
          onChange={(e) => setSourceId(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
        />
        <button onClick={handleSearch}>Search</button>
        <button onClick={openCreate}>+ Add New</button>
      </div>

      {error && <p className={styles.error}>{error}</p>}

      {loading ? (
        <p className={styles.loading}>Loading...</p>
      ) : (
        <table className={styles.table}>
          <thead>
            <tr>
              {columns.map((c) => (
                <th key={c.key}>{c.label}</th>
              ))}
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {items.length === 0 ? (
              <tr>
                <td colSpan={columns.length + 1} className={styles.empty}>
                  No data
                </td>
              </tr>
            ) : (
              items.map((s) => (
                <tr key={s.id}>
                  {columns.map((c) => (
                    <td key={c.key}>{(s as Record<string, unknown>)[c.key] as React.ReactNode}</td>
                  ))}
                  <td className={styles.actions}>
                    <button onClick={() => openEdit(s)}>Edit</button>
                    <button onClick={() => handleDelete(s.id)}>Delete</button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      )}

      <div className={styles.pagination}>
        <button disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
          Prev
        </button>
        <span>
          Page {page} / {totalPages || 1} (Total: {total})
        </span>
        <button disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>
          Next
        </button>
      </div>

      <Modal open={modalOpen} title={editing ? 'Edit Translation' : 'Add Translation'} onClose={() => setModalOpen(false)}>
        <div className={styles.form}>
          <label>
            Source ID
            <input
              type="number"
              value={form.source_id}
              onChange={(e) => setForm({ ...form, source_id: Number(e.target.value) })}
            />
          </label>
          <label>
            Direction
            <select value={form.direction} onChange={(e) => setForm({ ...form, direction: e.target.value })}>
              {DIRECTIONS.filter((d) => d).map((d) => (
                <option key={d} value={d}>
                  {d}
                </option>
              ))}
            </select>
          </label>
          <label>
            Source Text
            <textarea value={form.source_text} onChange={(e) => setForm({ ...form, source_text: e.target.value })} rows={3} />
          </label>
          <label>
            Reference Translation
            <textarea value={form.reference_translation} onChange={(e) => setForm({ ...form, reference_translation: e.target.value })} rows={3} />
          </label>
          <label>
            Position
            <input
              type="number"
              value={form.position}
              onChange={(e) => setForm({ ...form, position: Number(e.target.value) })}
            />
          </label>
          <button className={styles.saveBtn} onClick={handleSave}>
            Save
          </button>
        </div>
      </Modal>
    </div>
  )
}
