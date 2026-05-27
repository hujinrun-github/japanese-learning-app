import { useState, useEffect, useCallback } from 'react'
import { adminFetch } from '@/api/client'
import Modal from '@/components/Modal/Modal'
import styles from './Grammar.module.css'

interface GrammarPoint {
  id: number
  name: string
  meaning: string
  conjunction_rule: string
  usage_note: string
  jlpt_level: string
  examples: { japanese: string; chinese: string }[]
  quiz_questions: Record<string, unknown>[]
}

const LEVELS = ['', 'N5', 'N4', 'N3', 'N2', 'N1']
const PAGE_SIZE = 20

const emptyForm = {
  name: '',
  meaning: '',
  conjunction_rule: '',
  usage_note: '',
  jlpt_level: 'N5',
  examples: '[]',
  quiz_questions: '[]',
}

export default function GrammarPage() {
  const [items, setItems] = useState<GrammarPoint[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [level, setLevel] = useState('')
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<GrammarPoint | null>(null)
  const [form, setForm] = useState(emptyForm)

  const totalPages = Math.ceil(total / PAGE_SIZE)

  const fetchItems = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const params = new URLSearchParams({ page: String(page), size: String(PAGE_SIZE) })
      if (level) params.set('level', level)
      if (search) params.set('search', search)
      const data = await adminFetch<{ items: GrammarPoint[]; total: number }>('GET', `/grammar?${params}`)
      setItems(data.items)
      setTotal(data.total)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }, [page, level, search])

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

  function openEdit(g: GrammarPoint) {
    setEditing(g)
    setForm({
      name: g.name,
      meaning: g.meaning,
      conjunction_rule: g.conjunction_rule ?? '',
      usage_note: g.usage_note ?? '',
      jlpt_level: g.jlpt_level,
      examples: JSON.stringify(g.examples ?? []),
      quiz_questions: JSON.stringify(g.quiz_questions ?? []),
    })
    setModalOpen(true)
  }

  async function handleSave() {
    setError('')
    try {
      let examples: { japanese: string; chinese: string }[] = []
      let quiz_questions: Record<string, unknown>[] = []
      try {
        examples = JSON.parse(form.examples)
      } catch {
        examples = []
      }
      try {
        quiz_questions = JSON.parse(form.quiz_questions)
      } catch {
        quiz_questions = []
      }
      const body = { ...form, examples, quiz_questions }
      if (editing) {
        await adminFetch('PUT', `/grammar/${editing.id}`, body)
      } else {
        await adminFetch('POST', '/grammar', body)
      }
      setModalOpen(false)
      fetchItems()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed')
    }
  }

  async function handleDelete(id: number) {
    if (!confirm('Delete this grammar point?')) return
    setError('')
    try {
      await adminFetch('DELETE', `/grammar/${id}`)
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
      const data = await adminFetch<{ inserted: number }>('POST', '/import/grammar', fd)
      alert(`Imported ${data.inserted} grammar points`)
      fetchItems()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed')
    }
    e.target.value = ''
  }

  const columns = [
    { key: 'id', label: 'ID' },
    { key: 'name', label: 'Name' },
    { key: 'meaning', label: 'Meaning' },
    { key: 'jlpt_level', label: 'Level' },
    { key: 'conjunction_rule', label: 'Conjunction' },
  ]

  return (
    <div className={styles.page}>
      <h2 className={styles.title}>Grammar</h2>

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
              items.map((g) => (
                <tr key={g.id}>
                  {columns.map((c) => (
                    <td key={c.key}>{(g as Record<string, unknown>)[c.key] as React.ReactNode}</td>
                  ))}
                  <td className={styles.actions}>
                    <button onClick={() => openEdit(g)}>Edit</button>
                    <button onClick={() => handleDelete(g.id)}>Delete</button>
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

      <Modal open={modalOpen} title={editing ? 'Edit Grammar Point' : 'Add Grammar Point'} onClose={() => setModalOpen(false)}>
        <div className={styles.form}>
          <label>
            Name
            <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </label>
          <label>
            Meaning
            <input value={form.meaning} onChange={(e) => setForm({ ...form, meaning: e.target.value })} />
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
            Conjunction Rule
            <input value={form.conjunction_rule} onChange={(e) => setForm({ ...form, conjunction_rule: e.target.value })} />
          </label>
          <label>
            Usage Note
            <textarea value={form.usage_note} onChange={(e) => setForm({ ...form, usage_note: e.target.value })} rows={3} />
          </label>
          <label>
            Examples (JSON)
            <textarea value={form.examples} onChange={(e) => setForm({ ...form, examples: e.target.value })} rows={4} />
          </label>
          <label>
            Quiz Questions (JSON)
            <textarea value={form.quiz_questions} onChange={(e) => setForm({ ...form, quiz_questions: e.target.value })} rows={4} />
          </label>
          <button className={styles.saveBtn} onClick={handleSave}>
            Save
          </button>
        </div>
      </Modal>
    </div>
  )
}
