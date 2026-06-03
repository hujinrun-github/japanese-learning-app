import { Fragment, useState, useEffect, useCallback } from 'react'
import { adminFetch } from '@/api/client'
import Modal from '@/components/Modal/Modal'
import { TTSConfigFields, defaultTTSConfig, getDefaultTTSConfig, type TTSConfig } from '@/components/TTSConfigFields/TTSConfigFields'
import { AudioRegenButton } from '@/components/AudioRegenButton/AudioRegenButton'
import styles from './Words.module.css'

interface Word {
  id: number; kanji_form: string; reading: string; meaning: string
  part_of_speech: string; jlpt_level: string
  examples: { japanese: string; chinese: string }[]; reading_type: string; audio_url: string
}

const LEVELS = ['', 'N5', 'N4', 'N3', 'N2', 'N1']
const LVL_CSS: Record<string, string> = { N5: 'adm-lvlN5', N4: 'adm-lvlN4', N3: 'adm-lvlN3', N2: 'adm-lvlN2', N1: 'adm-lvlN1' }
const PAGE_SIZE = 20

const POS_ICON: Record<string, string> = { '動詞': '🔴', '名詞': '🔵', '形容詞': '🟡', '副詞': '🟣', '助詞': '⚪' }
const POS_CSS: Record<string, string> = { '動詞': 'adm-tagVerb', '名詞': 'adm-tagNoun', '形容詞': 'adm-tagAdj', '副詞': 'adm-tagAdv' }

const emptyForm = { kanji_form: '', reading: '', meaning: '', part_of_speech: '', jlpt_level: 'N5', examples: '[]', reading_type: '', auto_fill: true, generate_examples: false }

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
  const [showTTSSettings, setShowTTSSettings] = useState(false)
  const [ttsConfig, setTTSConfig] = useState<TTSConfig>(defaultTTSConfig)
  const [expandedId, setExpandedId] = useState<number | null>(null)

  useEffect(() => { getDefaultTTSConfig().then(setTTSConfig) }, [])

  const totalPages = Math.ceil(total / PAGE_SIZE)

  const fetchItems = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const params = new URLSearchParams({ page: String(page), size: String(PAGE_SIZE) })
      if (level) params.set('level', level)
      if (search) params.set('search', search)
      const data = await adminFetch<{ items: Word[]; total: number }>('GET', `/words?${params}`)
      setItems(data.items); setTotal(data.total)
    } catch (err) { setError(err instanceof Error ? err.message : 'Failed to load') }
    finally { setLoading(false) }
  }, [page, level, search])

  useEffect(() => { fetchItems() }, [fetchItems])
  function handleSearch() { setPage(1) }
  function openCreate() { setEditing(null); setForm(emptyForm); setModalOpen(true) }
  function openEdit(w: Word) {
    setEditing(w)
    setForm({ kanji_form: w.kanji_form, reading: w.reading, meaning: w.meaning, part_of_speech: w.part_of_speech, jlpt_level: w.jlpt_level, examples: JSON.stringify(w.examples ?? []), reading_type: w.reading_type ?? '', auto_fill: false, generate_examples: false })
    setModalOpen(true)
  }

  async function handleSave() {
    setError('')
    try {
      let examples = []
      try { examples = JSON.parse(form.examples) } catch { examples = [] }
      const body = { ...form, examples }
      if (editing) { await adminFetch('PUT', `/words/${editing.id}`, body) }
      else { await adminFetch('POST', '/words', body) }
      setModalOpen(false); fetchItems()
    } catch (err) { setError(err instanceof Error ? err.message : 'Save failed') }
  }

  async function handleDelete(id: number) {
    if (!confirm('Delete this word?')) return
    setError('')
    try { await adminFetch('DELETE', `/words/${id}`); fetchItems() }
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
        fd.append('instructions', ttsConfig.instructions); fd.append('tts_force', String(ttsConfig.tts_force))
        fd.append('sbv_url', ttsConfig.sbv_url); fd.append('sbv_model', ttsConfig.sbv_model)
        fd.append('sbv_speaker', ttsConfig.sbv_speaker); fd.append('sbv_style', ttsConfig.sbv_style)
      }
      const data = await adminFetch<{ inserted: number }>('POST', '/import/words', fd)
      alert(`Imported ${data.inserted} words${ttsConfig.provider ? ' (audio generation started)' : ''}`)
      fetchItems()
    } catch (err) { setError(err instanceof Error ? err.message : 'Import failed') }
    e.target.value = ''
  }

  function toggleExpand(id: number) {
    setExpandedId(prev => prev === id ? null : id)
  }

  return (
    <div className="adm-page">
      <div className="adm-header">
        <div className="adm-titleRow">
          <h2 className={`adm-title ${styles.title}`}>Words</h2>
          <span className="adm-count">{total} words</span>
        </div>
      </div>
      <div className="adm-toolbar">
        <select value={level} onChange={e => { setLevel(e.target.value); setPage(1) }}>
          <option value="">All Levels</option>
          {LEVELS.filter(l => l).map(l => <option key={l} value={l}>JLPT {l}</option>)}
        </select>
        <input placeholder="Search kanji, reading, meaning..." value={search} onChange={e => setSearch(e.target.value)} onKeyDown={e => e.key === 'Enter' && handleSearch()} />
        <button className="adm-btn" onClick={handleSearch}>Search</button>
        <button className="adm-btn" onClick={openCreate}>+ Add New</button>
        <label className="adm-btnOutline">📥 Import<input type="file" accept=".json" onChange={handleImport} hidden /></label>
        <label className="adm-ttsToggle"><input type="checkbox" checked={showTTSSettings} onChange={e => { setShowTTSSettings(e.target.checked); if (!e.target.checked) setTTSConfig(defaultTTSConfig) }} />🎙 Audio</label>
      </div>
      {showTTSSettings && <TTSConfigFields config={ttsConfig} onChange={setTTSConfig} showForce />}
      {error && <p className="adm-error">{error}</p>}
      {loading ? <p className="adm-loading">Loading words...</p> : (
        <>
          <table className="adm-table">
            <thead><tr><th style={{width:'36px'}}></th><th style={{width:'50px'}}>ID</th><th>Kanji</th><th>Reading</th><th>Meaning</th><th>POS</th><th>Level</th><th style={{width:'90px'}}>Audio</th><th style={{width:'140px'}}>Actions</th></tr></thead>
            <tbody>
              {items.length === 0 ? <tr><td colSpan={9} className="adm-empty">No words found</td></tr> :
                items.map(w => (
                  <Fragment key={w.id}>
                    <tr className="adm-clickableRow" onClick={() => toggleExpand(w.id)} style={{ cursor: 'pointer' }}>
                      <td style={{ textAlign: 'center', fontSize: '11px', color: '#b8b0a8', transition: 'transform 0.2s', transform: expandedId === w.id ? 'rotate(90deg)' : 'none' }}>
                        ▶
                      </td>
                      <td className="adm-id">{w.id}</td>
                      <td className="adm-kanji">{w.kanji_form}</td>
                      <td className="adm-reading">{w.reading || '-'}</td>
                      <td className="adm-meaning" title={w.meaning}>{w.meaning}</td>
                      <td><span className={`adm-tag ${POS_CSS[w.part_of_speech] || 'adm-tagOther'}`}>{POS_ICON[w.part_of_speech] || ''} {w.part_of_speech}</span></td>
                      <td><span className={`adm-levelBadge ${LVL_CSS[w.jlpt_level] || ''}`}>{w.jlpt_level}</span></td>
                      <td onClick={e => e.stopPropagation()}>
                        <AudioRegenButton text={w.reading} audioUrl={w.audio_url ? `/audio/words/${w.audio_url}` : undefined} module="word" wordId={w.id} onRegenerated={() => fetchItems()} />
                      </td>
                      <td className="adm-actions" onClick={e => e.stopPropagation()}>
                        <button onClick={() => openEdit(w)}>✏️ Edit</button>
                        <button className="adm-btnDanger" onClick={() => handleDelete(w.id)}>🗑 Delete</button>
                      </td>
                    </tr>
                    {/* Expandable examples row */}
                    {expandedId === w.id && (
                      <tr key={`${w.id}-ex`} className="adm-expandedRow">
                        <td colSpan={9}>
                          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '10px' }}>
                            <strong style={{ fontSize: '13px', color: '#4d4842' }}>📝 Examples</strong>
                            <span style={{ fontSize: '11px', color: '#b8b0a8' }}>({w.examples?.length ?? 0} sentences)</span>
                          </div>
                          {!w.examples || w.examples.length === 0 ? (
                            <p style={{ fontSize: '13px', color: '#b8b0a8', margin: 0 }}>No examples</p>
                          ) : (
                            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                              {w.examples.map((ex, i) => (
                                <div key={i} style={{
                                  display: 'flex', alignItems: 'center', gap: '10px',
                                  padding: '10px 14px', background: '#fff', borderRadius: '8px',
                                  border: '1px solid #e8e4e0',
                                }}>
                                  <span style={{
                                    fontSize: '11px', fontWeight: 700, color: '#b8b0a8',
                                    minWidth: '22px', textAlign: 'center',
                                  }}>{i + 1}</span>
                                  <div style={{ flex: 1, minWidth: 0 }}>
                                    <div style={{ fontSize: '14px', color: '#3d3833', lineHeight: '1.6', marginBottom: '2px' }}>
                                      {ex.japanese}
                                    </div>
                                    <div style={{ fontSize: '12px', color: '#8b8682' }}>
                                      {ex.chinese}
                                    </div>
                                  </div>
                                  <AudioRegenButton
                                    text={ex.japanese}
                                    module="example"
                                    onRegenerated={() => fetchItems()}
                                  />
                                </div>
                              ))}
                            </div>
                          )}
                        </td>
                      </tr>
                    )}
                  </Fragment>
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
      <Modal open={modalOpen} title={editing ? 'Edit Word' : 'Add Word'} onClose={() => setModalOpen(false)}>
        <div className="adm-form">
          <label>Kanji Form <input value={form.kanji_form} onChange={e => setForm({...form, kanji_form: e.target.value})} /></label>
          <label>Reading <input value={form.reading} onChange={e => setForm({...form, reading: e.target.value})} /></label>
          <label>Meaning <input value={form.meaning} onChange={e => setForm({...form, meaning: e.target.value})} /></label>
          <label>Part of Speech <input value={form.part_of_speech} onChange={e => setForm({...form, part_of_speech: e.target.value})} /></label>
          <label>JLPT Level <select value={form.jlpt_level} onChange={e => setForm({...form, jlpt_level: e.target.value})}>{LEVELS.filter(l => l).map(l => <option key={l} value={l}>{l}</option>)}</select></label>
          <label>Reading Type <input value={form.reading_type} onChange={e => setForm({...form, reading_type: e.target.value})} /></label>
          <label>Examples (JSON) <textarea value={form.examples} onChange={e => setForm({...form, examples: e.target.value})} rows={4} /></label>
          {!editing && <label className="adm-checkLabel"><input type="checkbox" checked={form.auto_fill} onChange={e => setForm({...form, auto_fill: e.target.checked})} />Auto-fill reading/POS (kagome)</label>}
          <label className="adm-checkLabel"><input type="checkbox" checked={form.generate_examples} onChange={e => setForm({...form, generate_examples: e.target.checked})} />Generate examples via AI (requires AI_API_KEY)</label>
          <button className="adm-saveBtn" onClick={handleSave}>Save</button>
        </div>
      </Modal>
    </div>
  )
}
