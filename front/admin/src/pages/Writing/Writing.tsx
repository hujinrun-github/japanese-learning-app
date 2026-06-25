import { useState, useEffect, useCallback } from 'react'
import { adminFetch } from '@/api/client'
import Modal from '@/components/Modal/Modal'
import styles from './Writing.module.css'

interface WritingQuestion {
  id: number; type: string; prompt: string; expected_answer: string; jlpt_level: string; grammar_point_id: number | null
}

const LEVELS = ['', 'N5', 'N4', 'N3', 'N2', 'N1']
const LVL_CSS: Record<string, string> = { N5: 'adm-lvlN5', N4: 'adm-lvlN4', N3: 'adm-lvlN3', N2: 'adm-lvlN2', N1: 'adm-lvlN1' }
const TYPES = ['input', 'sentence']
const TYPE_ICONS: Record<string, string> = { input: '⌨️', sentence: '📝' }
const PAGE_SIZE = 20

const emptyForm = { type: 'input', prompt: '', expected_answer: '', jlpt_level: 'N5', grammar_point_id: '' }

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
    setLoading(true); setError('')
    try {
      const params = new URLSearchParams({ page: String(page), size: String(PAGE_SIZE) })
      if (level) params.set('level', level)
      if (type) params.set('type', type)
      if (search) params.set('search', search)
      const data = await adminFetch<{ items: WritingQuestion[]; total: number }>('GET', `/writing?${params}`)
      setItems(data.items || []); setTotal(data.total || 0)
    } catch (err) { setError(err instanceof Error ? err.message : 'Failed to load') }
    finally { setLoading(false) }
  }, [page, level, type, search])

  useEffect(() => { fetchItems() }, [fetchItems])
  function handleSearch() { setPage(1) }
  function openCreate() { setEditing(null); setForm(emptyForm); setModalOpen(true) }
  function openEdit(q: WritingQuestion) {
    setEditing(q)
    setForm({ type: q.type, prompt: q.prompt, expected_answer: q.expected_answer ?? '', jlpt_level: q.jlpt_level, grammar_point_id: q.grammar_point_id !== null && q.grammar_point_id !== undefined ? String(q.grammar_point_id) : '' })
    setModalOpen(true)
  }

  async function handleSave() {
    setError('')
    try {
      const grammar_point_id = form.grammar_point_id ? parseInt(form.grammar_point_id, 10) : null
      const body = { type: form.type, prompt: form.prompt, expected_answer: form.expected_answer, jlpt_level: form.jlpt_level, grammar_point_id }
      if (editing) { await adminFetch('PUT', `/writing/${editing.id}`, body) }
      else { await adminFetch('POST', '/writing', body) }
      setModalOpen(false); fetchItems()
    } catch (err) { setError(err instanceof Error ? err.message : 'Save failed') }
  }

  async function handleDelete(id: number) {
    if (!confirm('Delete this writing question?')) return
    setError('')
    try { await adminFetch('DELETE', `/writing/${id}`); fetchItems() }
    catch (err) { setError(err instanceof Error ? err.message : 'Delete failed') }
  }

  async function handleImport(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]; if (!file) return
    setError('')
    try {
      const fd = new FormData(); fd.append('file', file)
      const data = await adminFetch<{ inserted: number }>('POST', '/import/writing', fd)
      alert(`Imported ${data.inserted} writing questions`)
      fetchItems()
    } catch (err) { setError(err instanceof Error ? err.message : 'Import failed') }
    e.target.value = ''
  }

  return (
    <div className="adm-page">
      <div className="adm-header">
        <div className="adm-titleRow">
          <h2 className={`adm-title ${styles.title}`}>Writing</h2>
          <span className="adm-count">{total} questions</span>
        </div>
      </div>
      <div className="adm-toolbar">
        <select value={level} onChange={e => { setLevel(e.target.value); setPage(1) }}>
          {LEVELS.map(l => <option key={l} value={l}>{l || 'All Levels'}</option>)}
        </select>
        <select value={type} onChange={e => { setType(e.target.value); setPage(1) }}>
          <option value="">All Types</option>
          {TYPES.map(t => <option key={t} value={t}>{TYPE_ICONS[t]} {t}</option>)}
        </select>
        <input placeholder="Search prompts..." value={search} onChange={e => setSearch(e.target.value)} onKeyDown={e => e.key === 'Enter' && handleSearch()} />
        <button className="adm-btn" onClick={handleSearch}>Search</button>
        <button className="adm-btn" onClick={openCreate}>+ Add New</button>
        <label className="adm-btnOutline">📥 Import<input type="file" accept=".json" onChange={handleImport} hidden /></label>
      </div>
      {error && <p className="adm-error">{error}</p>}
      {loading ? <p className="adm-loading">Loading writing questions...</p> : (
        <>
          <table className="adm-table">
            <thead><tr><th>ID</th><th>Type</th><th>Prompt</th><th>Expected Answer</th><th>Level</th><th>Grammar ID</th><th>Actions</th></tr></thead>
            <tbody>
              {items.length === 0 ? <tr><td colSpan={7} className="adm-empty">No questions found</td></tr> :
                items.map(q => (
                  <tr key={q.id}>
                    <td className="adm-id">{q.id}</td>
                    <td><span className="adm-typeBadge">{TYPE_ICONS[q.type] || ''} {q.type}</span></td>
                    <td className="adm-textCell" title={q.prompt}>{q.prompt}</td>
                    <td className="adm-textCell" title={q.expected_answer}>{q.expected_answer || '-'}</td>
                    <td><span className={`adm-levelBadge ${LVL_CSS[q.jlpt_level] || ''}`}>{q.jlpt_level}</span></td>
                    <td>{q.grammar_point_id ?? '-'}</td>
                    <td className="adm-actions">
                      <button onClick={() => openEdit(q)}>✏️ Edit</button>
                      <button className="adm-btnDanger" onClick={() => handleDelete(q.id)}>🗑 Delete</button>
                    </td>
                  </tr>
                ))
              }
            </tbody>
          </table>
          <div className="adm-pagination">
            <button disabled={page <= 1} onClick={() => setPage(p => p - 1)}>← Prev</button>
            <span className="adm-pageInfo">Page <strong>{page}</strong> / <strong>{totalPages || 1}</strong> <span style={{marginLeft:8,color:'#b8b0a8'}}>·</span> Total <strong>{total}</strong></span>
            <button disabled={page >= totalPages} onClick={() => setPage(p => p + 1)}>Next →</button>
          </div>
        </>
      )}
      <Modal open={modalOpen} title={editing ? 'Edit Writing Question' : 'Add Writing Question'} onClose={() => setModalOpen(false)}>
        <div className="adm-form">
          <label>Type <select value={form.type} onChange={e => setForm({...form, type: e.target.value})}>{TYPES.map(t => <option key={t} value={t}>{t}</option>)}</select></label>
          <label>Prompt <textarea value={form.prompt} onChange={e => setForm({...form, prompt: e.target.value})} rows={3} /></label>
          <label>Expected Answer <textarea value={form.expected_answer} onChange={e => setForm({...form, expected_answer: e.target.value})} rows={3} /></label>
          <label>JLPT Level <select value={form.jlpt_level} onChange={e => setForm({...form, jlpt_level: e.target.value})}>{LEVELS.filter(l => l).map(l => <option key={l} value={l}>{l}</option>)}</select></label>
          <label>Grammar Point ID <input type="number" value={form.grammar_point_id} onChange={e => setForm({...form, grammar_point_id: e.target.value})} placeholder="(optional)" /></label>
          <button className="adm-saveBtn" onClick={handleSave}>Save</button>
        </div>
      </Modal>
    </div>
  )
}
