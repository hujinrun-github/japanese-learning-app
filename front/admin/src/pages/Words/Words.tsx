import { useState, useEffect, useCallback } from 'react'
import { adminFetch } from '@/api/client'
import Modal from '@/components/Modal/Modal'
import styles from './Words.module.css'

interface Word {
  id: number
  kanji_form: string
  reading: string
  meaning: string
  part_of_speech: string
  jlpt_level: string
  examples: { japanese: string; chinese: string }[]
  reading_type: string
}

const LEVELS = ['', 'N5', 'N4', 'N3', 'N2', 'N1']
const PAGE_SIZE = 20

const emptyForm = {
  kanji_form: '',
  reading: '',
  meaning: '',
  part_of_speech: '',
  jlpt_level: 'N5',
  examples: '[]',
  reading_type: '',
}

export default function WordsPage() {
  const [items, setItems] = useState<Word[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [level, setLevel] = useState('')
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<Word | null>(null)
  const [form, setForm] = useState(emptyForm)

  const totalPages = Math.ceil(total / PAGE_SIZE)

  const fetchItems = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const params = new URLSearchParams({ page: String(page), size: String(PAGE_SIZE) })
      if (level) params.set('level', level)
      if (search) params.set('search', search)
      const data = await adminFetch<{ items: Word[]; total: number }>('GET', `/words?${params}`)
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

  function openEdit(w: Word) {
    setEditing(w)
    setForm({
      kanji_form: w.kanji_form,
      reading: w.reading,
      meaning: w.meaning,
      part_of_speech: w.part_of_speech,
      jlpt_level: w.jlpt_level,
      examples: JSON.stringify(w.examples ?? []),
      reading_type: w.reading_type ?? '',
    })
    setModalOpen(true)
  }

  async function handleSave() {
    setError('')
    try {
      let examples: { japanese: string; chinese: string }[] = []
      try {
        examples = JSON.parse(form.examples)
      } catch {
        examples = []
      }
      const body = { ...form, examples }
      if (editing) {
        await adminFetch('PUT', `/words/${editing.id}`, body)
      } else {
        await adminFetch('POST', '/words', body)
      }
      setModalOpen(false)
      fetchItems()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed')
    }
  }

  async function handleDelete(id: number) {
    if (!confirm('Delete this word?')) return
    setError('')
    try {
      await adminFetch('DELETE', `/words/${id}`)
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
      const data = await adminFetch<{ inserted: number }>('POST', '/import/words', fd)
      alert(`Imported ${data.inserted} words`)
      fetchItems()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed')
    }
    // Reset file input
    e.target.value = ''
  }

  const columns = [
    { key: 'id', label: 'ID' },
    { key: 'kanji_form', label: 'Kanji' },
    { key: 'reading', label: 'Reading' },
    { key: 'meaning', label: 'Meaning' },
    { key: 'part_of_speech', label: 'POS' },
    { key: 'jlpt_level', label: 'Level' },
  ]

  return (
    <div className={styles.page}>
      <h2 className={styles.title}>Words</h2>

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
              items.map((w) => (
                <tr key={w.id}>
                  {columns.map((c) => (
                    <td key={c.key}>{(w as Record<string, unknown>)[c.key] as React.ReactNode}</td>
                  ))}
                  <td className={styles.actions}>
                    <button onClick={() => openEdit(w)}>Edit</button>
                    <button onClick={() => handleDelete(w.id)}>Delete</button>
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

      <Modal open={modalOpen} title={editing ? 'Edit Word' : 'Add Word'} onClose={() => setModalOpen(false)}>
        <div className={styles.form}>
          <label>
            Kanji Form
            <input value={form.kanji_form} onChange={(e) => setForm({ ...form, kanji_form: e.target.value })} />
          </label>
          <label>
            Reading
            <input value={form.reading} onChange={(e) => setForm({ ...form, reading: e.target.value })} />
          </label>
          <label>
            Meaning
            <input value={form.meaning} onChange={(e) => setForm({ ...form, meaning: e.target.value })} />
          </label>
          <label>
            Part of Speech
            <input value={form.part_of_speech} onChange={(e) => setForm({ ...form, part_of_speech: e.target.value })} />
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
            Reading Type
            <input value={form.reading_type} onChange={(e) => setForm({ ...form, reading_type: e.target.value })} />
          </label>
          <label>
            Examples (JSON)
            <textarea value={form.examples} onChange={(e) => setForm({ ...form, examples: e.target.value })} rows={4} />
          </label>
          <button className={styles.saveBtn} onClick={handleSave}>
            Save
          </button>
        </div>
      </Modal>
    </div>
  )
}
