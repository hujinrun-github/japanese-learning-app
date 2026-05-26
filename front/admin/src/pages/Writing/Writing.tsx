import { useState, useEffect, useCallback } from 'react'
import { adminFetch } from '@/api/client'
import Modal from '@/components/Modal/Modal'
import styles from './Writing.module.css'

interface WritingQuestion {
  id: number
  type: string
  prompt: string
  expected_answer: string
  jlpt_level: string
  grammar_point_id: number | null
}

const LEVELS = ['', 'N5', 'N4', 'N3', 'N2', 'N1']
const TYPES = ['input', 'sentence']
const PAGE_SIZE = 20

const emptyForm = {
  type: 'input',
  prompt: '',
  expected_answer: '',
  jlpt_level: 'N5',
  grammar_point_id: '',
}

export default function WritingPage() {
  const [items, setItems] = useState<WritingQuestion[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [level, setLevel] = useState('')
  const [type, setType] = useState('')
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<WritingQuestion | null>(null)
  const [form, setForm] = useState(emptyForm)

  const totalPages = Math.ceil(total / PAGE_SIZE)

  const fetchItems = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const params = new URLSearchParams({ page: String(page), size: String(PAGE_SIZE) })
      if (level) params.set('level', level)
      if (type) params.set('type', type)
      if (search) params.set('search', search)
      const data = await adminFetch<{ items: WritingQuestion[]; total: number }>('GET', `/writing-questions?${params}`)
      setItems(data.items)
      setTotal(data.total)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }, [page, level, type, search])

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

  function openEdit(q: WritingQuestion) {
    setEditing(q)
    setForm({
      type: q.type,
      prompt: q.prompt,
      expected_answer: q.expected_answer ?? '',
      jlpt_level: q.jlpt_level,
      grammar_point_id: q.grammar_point_id !== null && q.grammar_point_id !== undefined ? String(q.grammar_point_id) : '',
    })
    setModalOpen(true)
  }

  async function handleSave() {
    setError('')
    try {
      const grammar_point_id = form.grammar_point_id ? parseInt(form.grammar_point_id, 10) : null
      const body = {
        type: form.type,
        prompt: form.prompt,
        expected_answer: form.expected_answer,
        jlpt_level: form.jlpt_level,
        grammar_point_id,
      }
      if (editing) {
        await adminFetch('PUT', `/writing-questions/${editing.id}`, body)
      } else {
        await adminFetch('POST', '/writing-questions', body)
      }
      setModalOpen(false)
      fetchItems()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed')
    }
  }

  async function handleDelete(id: number) {
    if (!confirm('Delete this writing question?')) return
    setError('')
    try {
      await adminFetch('DELETE', `/writing-questions/${id}`)
      fetchItems()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed')
    }
  }

  async function handleImport(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    setError('')
    try {
      const fd = new FormData()
      fd.append('file', file)
      const data = await adminFetch<{ inserted: number }>('POST', '/import/writing', fd)
      alert(`Imported ${data.inserted} writing questions`)
      fetchItems()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed')
    }
    e.target.value = ''
  }

  const columns = [
    { key: 'id', label: 'ID' },
    { key: 'type', label: 'Type' },
    { key: 'prompt', label: 'Prompt' },
    { key: 'expected_answer', label: 'Expected Answer' },
    { key: 'jlpt_level', label: 'Level' },
    { key: 'grammar_point_id', label: 'Grammar ID' },
  ]

  return (
    <div className={styles.page}>
      <h2 className={styles.title}>Writing</h2>

      <div className={styles.toolbar}>
        <select
          value={level}
          onChange={(e) => {
            setLevel(e.target.value)
            setPage(1)
          }}
        >
          {LEVELS.map((l) => (
            <option key={l} value={l}>
              {l || 'All Levels'}
            </option>
          ))}
        </select>
        <select
          value={type}
          onChange={(e) => {
            setType(e.target.value)
            setPage(1)
          }}
        >
          <option value="">All Types</option>
          {TYPES.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
        <input
          placeholder="Search..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
        />
        <button onClick={handleSearch}>Search</button>
        <button onClick={openCreate}>+ Add New</button>
        <label className={styles.importBtn}>
          Import
          <input type="file" accept=".json" onChange={handleImport} hidden />
        </label>
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
              items.map((q) => (
                <tr key={q.id}>
                  {columns.map((c) => (
                    <td key={c.key}>{(q as Record<string, unknown>)[c.key] as React.ReactNode}</td>
                  ))}
                  <td className={styles.actions}>
                    <button onClick={() => openEdit(q)}>Edit</button>
                    <button onClick={() => handleDelete(q.id)}>Delete</button>
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

      <Modal open={modalOpen} title={editing ? 'Edit Writing Question' : 'Add Writing Question'} onClose={() => setModalOpen(false)}>
        <div className={styles.form}>
          <label>
            Type
            <select value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value })}>
              {TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </label>
          <label>
            Prompt
            <textarea value={form.prompt} onChange={(e) => setForm({ ...form, prompt: e.target.value })} rows={3} />
          </label>
          <label>
            Expected Answer
            <textarea value={form.expected_answer} onChange={(e) => setForm({ ...form, expected_answer: e.target.value })} rows={3} />
          </label>
          <label>
            JLPT Level
            <select value={form.jlpt_level} onChange={(e) => setForm({ ...form, jlpt_level: e.target.value })}>
              {LEVELS.filter((l) => l).map((l) => (
                <option key={l} value={l}>
                  {l}
                </option>
              ))}
            </select>
          </label>
          <label>
            Grammar Point ID
            <input value={form.grammar_point_id} onChange={(e) => setForm({ ...form, grammar_point_id: e.target.value })} placeholder="(optional)" />
          </label>
          <button className={styles.saveBtn} onClick={handleSave}>
            Save
          </button>
        </div>
      </Modal>
    </div>
  )
}
