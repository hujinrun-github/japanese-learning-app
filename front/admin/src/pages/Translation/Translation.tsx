import { useState, useEffect, useCallback } from 'react'
import { adminFetch } from '@/api/client'
import Modal from '@/components/Modal/Modal'
import styles from './Translation.module.css'

interface TranslationSentence {
  id: number; source_id: number; direction: string; source_text: string; reference_translation: string; position: number
}

const DIRECTIONS = ['', 'cn2jp', 'jp2cn']
const DIR_ICONS: Record<string, string> = { cn2jp: '🇨🇳→🇯🇵', jp2cn: '🇯🇵→🇨🇳' }
const DIR_CSS: Record<string, string> = { cn2jp: 'adm-dirC2J', jp2cn: 'adm-dirJ2C' }
const PAGE_SIZE = 20

const emptyForm = { source_id: 0, direction: 'cn2jp', source_text: '', reference_translation: '', position: 0 }

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
    setLoading(true); setError('')
    try {
      const params = new URLSearchParams({ page: String(page), size: String(PAGE_SIZE) })
      if (sourceId) params.set('source_id', sourceId)
      if (direction) params.set('direction', direction)
      const data = await adminFetch<{ items: TranslationSentence[]; total: number }>('GET', `/translation?${params}`)
      setItems(data.items || []); setTotal(data.total || 0)
    } catch (err) { setError(err instanceof Error ? err.message : 'Failed to load') }
    finally { setLoading(false) }
  }, [page, sourceId, direction])

  useEffect(() => { fetchItems() }, [fetchItems])
  function handleSearch() { setPage(1) }
  function openCreate() { setEditing(null); setForm(emptyForm); setModalOpen(true) }
  function openEdit(s: TranslationSentence) {
    setEditing(s)
    setForm({ source_id: s.source_id, direction: s.direction, source_text: s.source_text, reference_translation: s.reference_translation, position: s.position })
    setModalOpen(true)
  }

  async function handleSave() {
    setError('')
    try {
      const body = { source_id: Number(form.source_id), direction: form.direction, source_text: form.source_text, reference_translation: form.reference_translation, position: Number(form.position) }
      if (editing) { await adminFetch('PUT', `/translation/${editing.id}`, body) }
      else { await adminFetch('POST', '/translation', body) }
      setModalOpen(false); fetchItems()
    } catch (err) { setError(err instanceof Error ? err.message : 'Save failed') }
  }

  async function handleDelete(id: number) {
    if (!confirm('Delete this translation sentence?')) return
    setError('')
    try { await adminFetch('DELETE', `/translation/${id}`); fetchItems() }
    catch (err) { setError(err instanceof Error ? err.message : 'Delete failed') }
  }

  return (
    <div className="adm-page">
      <div className="adm-header">
        <div className="adm-titleRow">
          <h2 className={`adm-title ${styles.title}`}>Translation</h2>
          <span className="adm-count">{total} sentences</span>
        </div>
      </div>
      <div className="adm-toolbar">
        <select value={direction} onChange={e => { setDirection(e.target.value); setPage(1) }}>
          {DIRECTIONS.map(d => <option key={d} value={d}>{d || 'All Directions'}</option>)}
        </select>
        <input type="number" placeholder="Source ID..." value={sourceId} onChange={e => setSourceId(e.target.value)} onKeyDown={e => e.key === 'Enter' && handleSearch()} />
        <button className="adm-btn" onClick={handleSearch}>Search</button>
        <button className="adm-btn" onClick={openCreate}>+ Add New</button>
      </div>
      {error && <p className="adm-error">{error}</p>}
      {loading ? <p className="adm-loading">Loading translation sentences...</p> : (
        <>
          <table className="adm-table">
            <thead><tr><th>ID</th><th>Source ID</th><th>Direction</th><th>Source Text</th><th>Reference</th><th>Pos</th><th>Actions</th></tr></thead>
            <tbody>
              {items.length === 0 ? <tr><td colSpan={7} className="adm-empty">No sentences found</td></tr> :
                items.map(s => (
                  <tr key={s.id}>
                    <td className="adm-id">{s.id}</td>
                    <td>{s.source_id}</td>
                    <td><span className={`adm-typeBadge ${DIR_CSS[s.direction] || ''}`}>{DIR_ICONS[s.direction] || ''} {s.direction}</span></td>
                    <td className="adm-textCell" title={s.source_text}>{s.source_text}</td>
                    <td className="adm-textCell" title={s.reference_translation}>{s.reference_translation}</td>
                    <td className="adm-id">{s.position}</td>
                    <td className="adm-actions">
                      <button onClick={() => openEdit(s)}>✏️ Edit</button>
                      <button className="adm-btnDanger" onClick={() => handleDelete(s.id)}>🗑 Delete</button>
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
      <Modal open={modalOpen} title={editing ? 'Edit Translation' : 'Add Translation'} onClose={() => setModalOpen(false)}>
        <div className="adm-form">
          <label>Source ID <input type="number" value={form.source_id} onChange={e => setForm({...form, source_id: Number(e.target.value)})} /></label>
          <label>Direction <select value={form.direction} onChange={e => setForm({...form, direction: e.target.value})}>{DIRECTIONS.filter(d => d).map(d => <option key={d} value={d}>{d}</option>)}</select></label>
          <label>Source Text <textarea value={form.source_text} onChange={e => setForm({...form, source_text: e.target.value})} rows={3} /></label>
          <label>Reference Translation <textarea value={form.reference_translation} onChange={e => setForm({...form, reference_translation: e.target.value})} rows={3} /></label>
          <label>Position <input type="number" value={form.position} onChange={e => setForm({...form, position: Number(e.target.value)})} /></label>
          <button className="adm-saveBtn" onClick={handleSave}>Save</button>
        </div>
      </Modal>
    </div>
  )
}
