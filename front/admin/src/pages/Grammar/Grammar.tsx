import { useState, useEffect, useCallback } from 'react'
import { adminFetch } from '@/api/client'
import Modal from '@/components/Modal/Modal'
import { TTSConfigFields, defaultTTSConfig, getDefaultTTSConfig, type TTSConfig } from '@/components/TTSConfigFields/TTSConfigFields'
import { AudioRegenButton } from '@/components/AudioRegenButton/AudioRegenButton'
import { buildBatchAudioPayload, formatBatchAudioResult, type BatchAudioResponse } from '@/util/ttsBatch'
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
const LVL_CSS: Record<string, string> = { N5: 'adm-lvlN5', N4: 'adm-lvlN4', N3: 'adm-lvlN3', N2: 'adm-lvlN2', N1: 'adm-lvlN1' }
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
  const [showTTSSettings, setShowTTSSettings] = useState(false)
  const [ttsConfig, setTTSConfig] = useState<TTSConfig>(defaultTTSConfig)
  const [batchingAudio, setBatchingAudio] = useState(false)
  const [selectedIds, setSelectedIds] = useState<Set<number>>(() => new Set())

  useEffect(() => { getDefaultTTSConfig().then(setTTSConfig) }, [])

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

  useEffect(() => { fetchItems() }, [fetchItems])

  function handleSearch() { setPage(1); setSelectedIds(new Set()) }

  function openCreate() { setEditing(null); setForm(emptyForm); setModalOpen(true) }

  function openEdit(g: GrammarPoint) {
    setEditing(g)
    setForm({
      name: g.name, meaning: g.meaning,
      conjunction_rule: g.conjunction_rule ?? '', usage_note: g.usage_note ?? '',
      jlpt_level: g.jlpt_level,
      examples: JSON.stringify(g.examples ?? []), quiz_questions: JSON.stringify(g.quiz_questions ?? []),
    })
    setModalOpen(true)
  }

  async function handleSave() {
    setError('')
    try {
      let examples = [], quiz_questions = []
      try { examples = JSON.parse(form.examples) } catch { examples = [] }
      try { quiz_questions = JSON.parse(form.quiz_questions) } catch { quiz_questions = [] }
      const body = { ...form, examples, quiz_questions }
      if (editing) { await adminFetch('PUT', `/grammar/${editing.id}`, body) }
      else { await adminFetch('POST', '/grammar', body) }
      setModalOpen(false); fetchItems()
    } catch (err) { setError(err instanceof Error ? err.message : 'Save failed') }
  }

  async function handleDelete(id: number) {
    if (!confirm('Delete this grammar point?')) return
    setError('')
    try { await adminFetch('DELETE', `/grammar/${id}`); fetchItems() }
    catch (err) { setError(err instanceof Error ? err.message : 'Delete failed') }
  }

  async function handleImport(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]; if (!file) return
    setError('')
    try {
      const fd = new FormData(); fd.append('file', file)
      if (ttsConfig.provider) {
        fd.append('tts_provider', ttsConfig.provider)
        fd.append('tts_url', ttsConfig.tts_url)
        fd.append('tts_model', ttsConfig.tts_model)
        fd.append('voice', ttsConfig.voice)
        fd.append('instructions', ttsConfig.instructions)
        fd.append('tts_force', String(ttsConfig.tts_force))
        fd.append('sbv_url', ttsConfig.sbv_url)
        fd.append('sbv_model', ttsConfig.sbv_model)
        fd.append('sbv_speaker', ttsConfig.sbv_speaker)
        fd.append('sbv_style', ttsConfig.sbv_style)
      }
      const data = await adminFetch<{ inserted: number }>('POST', '/import/grammar', fd)
      alert(`Imported ${data.inserted} grammar points${ttsConfig.provider ? ' (audio generation started)' : ''}`)
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
    const texts = selectedVisibleItems.map(g => g.examples?.[0]?.japanese || g.name).filter(Boolean)
    if (texts.length === 0) {
      setError('Select at least one visible grammar row with text')
      return
    }
    if (!confirm(`Regenerate audio for ${texts.length} selected grammar rows? Existing files will be ${ttsConfig.tts_force ? 'overwritten' : 'skipped'}.`)) return

    setBatchingAudio(true)
    setError('')
    try {
      const data = await adminFetch<BatchAudioResponse>('POST', '/audio/batch', buildBatchAudioPayload('grammar', ttsConfig, { texts }))
      alert(formatBatchAudioResult('Grammar', data))
      fetchItems()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Batch audio failed')
    } finally {
      setBatchingAudio(false)
    }
  }

  const selectableIds = items.map(g => g.id)
  const selectedVisibleItems = items.filter(g => selectedIds.has(g.id))
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
          <h2 className={`adm-title ${styles.title}`}>Grammar</h2>
          <span className="adm-count">{total} points</span>
        </div>
      </div>

      <div className="adm-toolbar">
        <select value={level} onChange={(e) => { setLevel(e.target.value); setPage(1); setSelectedIds(new Set()) }}>
          {LEVELS.map(l => <option key={l} value={l}>{l || 'All Levels'}</option>)}
        </select>
        <input placeholder="Search name, meaning..." value={search} onChange={e => setSearch(e.target.value)} onKeyDown={e => e.key === 'Enter' && handleSearch()} />
        <button className="adm-btn" onClick={handleSearch}>Search</button>
        <button className="adm-btn" onClick={openCreate}>+ Add New</button>
        <label className="adm-btnOutline">
          📥 Import
          <input type="file" accept=".json" onChange={handleImport} hidden />
        </label>
        <label className="adm-ttsToggle">
          <input type="checkbox" checked={showTTSSettings} onChange={e => { setShowTTSSettings(e.target.checked); if (!e.target.checked) setTTSConfig(defaultTTSConfig) }} />
          🎙 Audio
        </label>
        {(showTTSSettings || selectedVisibleItems.length > 0) && <button className="adm-btnOutline" onClick={handleBatchAudio} disabled={batchingAudio || selectedVisibleItems.length === 0}>{batchingAudio ? 'Generating...' : `Regenerate Selected Audio (${selectedVisibleItems.length})`}</button>}
      </div>

      {showTTSSettings && <TTSConfigFields config={ttsConfig} onChange={setTTSConfig} showForce />}
      {error && <p className="adm-error">{error}</p>}

      {loading ? <p className="adm-loading">Loading grammar points...</p> : (
        <>
          <table className="adm-table">
            <thead><tr><th style={{width:'36px'}}><input type="checkbox" checked={allVisibleSelected} disabled={selectableIds.length === 0} onChange={e => toggleSelectAllVisible(e.target.checked)} aria-label="Select visible grammar rows" /></th><th>ID</th><th>Name</th><th>Meaning</th><th>Level</th><th>Conjunction</th><th>Audio</th><th>Actions</th></tr></thead>
            <tbody>
              {items.length === 0 ? (
                <tr><td colSpan={8} className="adm-empty">No grammar points found</td></tr>
              ) : items.map(g => (
                <tr key={g.id}>
                  <td style={{ textAlign: 'center' }}>
                    <input
                      type="checkbox"
                      checked={selectedIds.has(g.id)}
                      onChange={e => toggleSelected(g.id, e.target.checked)}
                      aria-label={`Select grammar ${g.name}`}
                    />
                  </td>
                  <td className="adm-id">{g.id}</td>
                  <td className="adm-kanji">{g.name}</td>
                  <td className="adm-meaning" title={g.meaning}>{g.meaning}</td>
                  <td><span className={`adm-levelBadge ${LVL_CSS[g.jlpt_level] || ''}`}>{g.jlpt_level}</span></td>
                  <td className="adm-textCell" title={g.conjunction_rule}>{g.conjunction_rule || '-'}</td>
                  <td>
                    <AudioRegenButton text={g.examples?.[0]?.japanese || g.name} module="example" onRegenerated={() => fetchItems()} />
                  </td>
                  <td className="adm-actions">
                    <button onClick={() => openEdit(g)}>✏️ Edit</button>
                    <button className="adm-btnDanger" onClick={() => handleDelete(g.id)}>🗑 Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="adm-pagination">
            <button disabled={page <= 1} onClick={() => { setPage(p => p - 1); setSelectedIds(new Set()) }}>← Prev</button>
            <span className="adm-pageInfo">Page <strong>{page}</strong> / <strong>{totalPages || 1}</strong> <span style={{marginLeft:8,color:'#b8b0a8'}}>·</span> Total <strong>{total}</strong></span>
            <button disabled={page >= totalPages} onClick={() => { setPage(p => p + 1); setSelectedIds(new Set()) }}>Next →</button>
          </div>
        </>
      )}

      <Modal open={modalOpen} title={editing ? 'Edit Grammar Point' : 'Add Grammar Point'} onClose={() => setModalOpen(false)}>
        <div className="adm-form">
          <label>Name <input value={form.name} onChange={e => setForm({...form, name: e.target.value})} /></label>
          <label>Meaning <input value={form.meaning} onChange={e => setForm({...form, meaning: e.target.value})} /></label>
          <label>JLPT Level <select value={form.jlpt_level} onChange={e => setForm({...form, jlpt_level: e.target.value})}>{LEVELS.filter(l => l).map(l => <option key={l} value={l}>{l}</option>)}</select></label>
          <label>Conjunction Rule <input value={form.conjunction_rule} onChange={e => setForm({...form, conjunction_rule: e.target.value})} /></label>
          <label>Usage Note <textarea value={form.usage_note} onChange={e => setForm({...form, usage_note: e.target.value})} rows={3} /></label>
          <label>Examples (JSON) <textarea value={form.examples} onChange={e => setForm({...form, examples: e.target.value})} rows={4} /></label>
          <label>Quiz Questions (JSON) <textarea value={form.quiz_questions} onChange={e => setForm({...form, quiz_questions: e.target.value})} rows={4} /></label>
          <button className="adm-saveBtn" onClick={handleSave}>Save</button>
        </div>
      </Modal>
    </div>
  )
}
