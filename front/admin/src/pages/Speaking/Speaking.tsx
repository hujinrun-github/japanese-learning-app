import { useState, useEffect, useCallback } from 'react'
import { adminFetch } from '@/api/client'
import Modal from '@/components/Modal/Modal'
import { TTSConfigFields, defaultTTSConfig, getDefaultTTSConfig, type TTSConfig } from '@/components/TTSConfigFields/TTSConfigFields'
import { AudioRegenButton } from '@/components/AudioRegenButton/AudioRegenButton'
import { buildBatchAudioPayload, formatBatchAudioResult, type BatchAudioResponse } from '@/util/ttsBatch'
import styles from './Speaking.module.css'

interface SpeakingQuestion {
  id: number; type: string; title: string; text: string; audio_url: string; jlpt_level: string
}

const LEVELS = ['', 'N5', 'N4', 'N3', 'N2', 'N1']
const LVL_CSS: Record<string, string> = { N5: 'adm-lvlN5', N4: 'adm-lvlN4', N3: 'adm-lvlN3', N2: 'adm-lvlN2', N1: 'adm-lvlN1' }
const TYPES = ['read_aloud', 'picture_description', 'free_talk', 'question_answer']
const TYPE_ICONS: Record<string, string> = { read_aloud: '📖', picture_description: '🖼', free_talk: '💬', question_answer: '❓' }
const PAGE_SIZE = 20

const emptyForm = { type: 'read_aloud', title: '', text: '', audio_url: '', jlpt_level: 'N5' }

export default function SpeakingPage() {
  const [items, setItems] = useState<SpeakingQuestion[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [level, setLevel] = useState('')
  const [type, setType] = useState('')
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<SpeakingQuestion | null>(null)
  const [form, setForm] = useState(emptyForm)
  const [showTTSSettings, setShowTTSSettings] = useState(false)
  const [ttsConfig, setTTSConfig] = useState<TTSConfig>(defaultTTSConfig)
  const [batchingAudio, setBatchingAudio] = useState(false)
  const [selectedIds, setSelectedIds] = useState<Set<number>>(() => new Set())

  useEffect(() => { getDefaultTTSConfig().then(setTTSConfig) }, [])

  const totalPages = Math.ceil(total / PAGE_SIZE)

  const fetchItems = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const params = new URLSearchParams({ page: String(page), size: String(PAGE_SIZE) })
      if (level) params.set('level', level)
      if (type) params.set('type', type)
      if (search) params.set('search', search)
      const data = await adminFetch<{ items: SpeakingQuestion[]; total: number }>('GET', `/speaking?${params}`)
      setItems(data.items || []); setTotal(data.total || 0)
    } catch (err) { setError(err instanceof Error ? err.message : 'Failed to load') }
    finally { setLoading(false) }
  }, [page, level, type, search])

  useEffect(() => { fetchItems() }, [fetchItems])
  function handleSearch() { setPage(1); setSelectedIds(new Set()) }
  function openCreate() { setEditing(null); setForm(emptyForm); setModalOpen(true) }
  function openEdit(q: SpeakingQuestion) {
    setEditing(q); setForm({ type: q.type, title: q.title, text: q.text ?? '', audio_url: q.audio_url ?? '', jlpt_level: q.jlpt_level })
    setModalOpen(true)
  }

  async function handleSave() {
    setError('')
    try {
      const body = { ...form }
      if (editing) { await adminFetch('PUT', `/speaking/${editing.id}`, body) }
      else { await adminFetch('POST', '/speaking', body) }
      setModalOpen(false); fetchItems()
    } catch (err) { setError(err instanceof Error ? err.message : 'Save failed') }
  }

  async function handleDelete(id: number) {
    if (!confirm('Delete this speaking question?')) return
    setError('')
    try { await adminFetch('DELETE', `/speaking/${id}`); fetchItems() }
    catch (err) { setError(err instanceof Error ? err.message : 'Delete failed') }
  }

  async function handleImport(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]; if (!file) return
    setError('')
    try {
      const fd = new FormData(); fd.append('file', file)
      if (ttsConfig.provider) {
        fd.append('tts_provider', ttsConfig.provider); fd.append('tts_url', ttsConfig.tts_url)
        fd.append('tts_model', ttsConfig.tts_model); fd.append('voice', ttsConfig.voice)
        fd.append('instructions', ttsConfig.instructions)
        fd.append('tts_force', String(ttsConfig.tts_force))
        fd.append('sbv_url', ttsConfig.sbv_url); fd.append('sbv_model', ttsConfig.sbv_model)
        fd.append('sbv_speaker', ttsConfig.sbv_speaker); fd.append('sbv_style', ttsConfig.sbv_style)
      }
      const data = await adminFetch<{ inserted: number }>('POST', '/import/speaking', fd)
      alert(`Imported ${data.inserted} speaking questions${ttsConfig.provider ? ' (audio generation started)' : ''}`)
      fetchItems()
    } catch (err) { setError(err instanceof Error ? err.message : 'Import failed') }
    e.target.value = ''
  }

  async function handleBatchAudio() {
    if (!ttsConfig.provider) {
      setShowTTSSettings(true)
      setError('Select a TTS provider first')
      return
    }
    const texts = selectedVisibleItems.map(q => q.text).filter(Boolean)
    if (texts.length === 0) {
      setError('Select at least one visible speaking row with text')
      return
    }
    if (!confirm(`Regenerate audio for ${texts.length} selected speaking rows? Existing files will be ${ttsConfig.tts_force ? 'overwritten' : 'skipped'}.`)) return

    setBatchingAudio(true)
    setError('')
    try {
      const data = await adminFetch<BatchAudioResponse>('POST', '/audio/batch', buildBatchAudioPayload('speaking', ttsConfig, { texts }))
      alert(formatBatchAudioResult('Speaking', data))
      fetchItems()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Batch audio failed')
    } finally {
      setBatchingAudio(false)
    }
  }

  const selectableIds = (items || []).filter(q => q.text).map(q => q.id)
  const selectedVisibleItems = (items || []).filter(q => q.text && selectedIds.has(q.id))
  const allVisibleSelected = selectableIds.length > 0 && selectedVisibleItems.length === selectableIds.length

  function toggleSelectAllVisible(checked: boolean) {
    setSelectedIds(prev => {
      const next = new Set(prev)
      selectableIds.forEach(id => checked ? next.add(id) : next.delete(id))
      return next
    })
  }

  function toggleSelected(id: number, checked: boolean) {
    setSelectedIds(prev => {
      const next = new Set(prev)
      if (checked) next.add(id)
      else next.delete(id)
      return next
    })
  }

  return (
    <div className="adm-page">
      <div className="adm-header">
        <div className="adm-titleRow">
          <h2 className={`adm-title ${styles.title}`}>Speaking</h2>
          <span className="adm-count">{total} questions</span>
        </div>
      </div>
      <div className="adm-toolbar">
        <select value={level} onChange={e => { setLevel(e.target.value); setPage(1); setSelectedIds(new Set()) }}>
          {LEVELS.map(l => <option key={l} value={l}>{l || 'All Levels'}</option>)}
        </select>
        <select value={type} onChange={e => { setType(e.target.value); setPage(1); setSelectedIds(new Set()) }}>
          <option value="">All Types</option>
          {TYPES.map(t => <option key={t} value={t}>{TYPE_ICONS[t]} {t}</option>)}
        </select>
        <input placeholder="Search title, text..." value={search} onChange={e => setSearch(e.target.value)} onKeyDown={e => e.key === 'Enter' && handleSearch()} />
        <button className="adm-btn" onClick={handleSearch}>Search</button>
        <button className="adm-btn" onClick={openCreate}>+ Add New</button>
        <label className="adm-btnOutline">📥 Import<input type="file" accept=".json" onChange={handleImport} hidden /></label>
        <label className="adm-ttsToggle"><input type="checkbox" checked={showTTSSettings} onChange={e => { setShowTTSSettings(e.target.checked); if (!e.target.checked) setTTSConfig(defaultTTSConfig) }} />🎙 Audio</label>
        {(showTTSSettings || selectedVisibleItems.length > 0) && <button className="adm-btnOutline" onClick={handleBatchAudio} disabled={batchingAudio || selectedVisibleItems.length === 0}>{batchingAudio ? 'Generating...' : `Regenerate Selected Audio (${selectedVisibleItems.length})`}</button>}
      </div>
      {showTTSSettings && <TTSConfigFields config={ttsConfig} onChange={setTTSConfig} showForce />}
      {error && <p className="adm-error">{error}</p>}
      {loading ? <p className="adm-loading">Loading speaking questions...</p> : (
        <>
          <table className="adm-table">
            <thead><tr><th style={{width:'36px'}}><input type="checkbox" checked={allVisibleSelected} disabled={selectableIds.length === 0} onChange={e => toggleSelectAllVisible(e.target.checked)} aria-label="Select visible speaking rows" /></th><th>ID</th><th>Type</th><th>Title</th><th>Text</th><th>Level</th><th>Audio</th><th>Actions</th></tr></thead>
            <tbody>
              {items.length === 0 ? <tr><td colSpan={8} className="adm-empty">No questions found</td></tr> :
                items.map(q => (
                  <tr key={q.id}>
                    <td style={{ textAlign: 'center' }}>
                      <input
                        type="checkbox"
                        checked={selectedIds.has(q.id)}
                        disabled={!q.text}
                        onChange={e => toggleSelected(q.id, e.target.checked)}
                        aria-label={`Select speaking ${q.title}`}
                      />
                    </td>
                    <td className="adm-id">{q.id}</td>
                    <td><span className="adm-typeBadge">{TYPE_ICONS[q.type] || ''} {q.type}</span></td>
                    <td className="adm-kanji">{q.title}</td>
                    <td className="adm-textCell" title={q.text}>{q.text || '-'}</td>
                    <td><span className={`adm-levelBadge ${LVL_CSS[q.jlpt_level] || ''}`}>{q.jlpt_level}</span></td>
                    <td><AudioRegenButton text={q.text} audioUrl={q.audio_url?.startsWith('http') ? q.audio_url : undefined} module="example" onRegenerated={() => fetchItems()} /></td>
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
            <button disabled={page <= 1} onClick={() => { setPage(p => p - 1); setSelectedIds(new Set()) }}>← Prev</button>
            <span className="adm-pageInfo">Page <strong>{page}</strong> / <strong>{totalPages || 1}</strong> <span style={{marginLeft:8,color:'#b8b0a8'}}>·</span> Total <strong>{total}</strong></span>
            <button disabled={page >= totalPages} onClick={() => { setPage(p => p + 1); setSelectedIds(new Set()) }}>Next →</button>
          </div>
        </>
      )}
      <Modal open={modalOpen} title={editing ? 'Edit Speaking Question' : 'Add Speaking Question'} onClose={() => setModalOpen(false)}>
        <div className="adm-form">
          <label>Type <select value={form.type} onChange={e => setForm({...form, type: e.target.value})}>{TYPES.map(t => <option key={t} value={t}>{t}</option>)}</select></label>
          <label>Title <input value={form.title} onChange={e => setForm({...form, title: e.target.value})} /></label>
          <label>JLPT Level <select value={form.jlpt_level} onChange={e => setForm({...form, jlpt_level: e.target.value})}>{LEVELS.filter(l => l).map(l => <option key={l} value={l}>{l}</option>)}</select></label>
          <label>Text <textarea value={form.text} onChange={e => setForm({...form, text: e.target.value})} rows={4} /></label>
          <label>Audio URL <input value={form.audio_url} onChange={e => setForm({...form, audio_url: e.target.value})} placeholder="https://..." /></label>
          <button className="adm-saveBtn" onClick={handleSave}>Save</button>
        </div>
      </Modal>
    </div>
  )
}
