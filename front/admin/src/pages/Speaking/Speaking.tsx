import { useState, useEffect, useCallback } from 'react'
import { adminFetch } from '@/api/client'
import Modal from '@/components/Modal/Modal'
import { TTSConfigFields, defaultTTSConfig, type TTSConfig } from '@/components/TTSConfigFields/TTSConfigFields'
import { AudioRegenButton } from '@/components/AudioRegenButton/AudioRegenButton'
import styles from './Speaking.module.css'

interface SpeakingQuestion {
  id: number
  type: string
  title: string
  text: string
  audio_url: string
  jlpt_level: string
}

const LEVELS = ['', 'N5', 'N4', 'N3', 'N2', 'N1']
const TYPES = ['read_aloud', 'picture_description', 'free_talk', 'question_answer']
const PAGE_SIZE = 20

const emptyForm = {
  type: 'read_aloud',
  title: '',
  text: '',
  audio_url: '',
  jlpt_level: 'N5',
}

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

  const totalPages = Math.ceil(total / PAGE_SIZE)

  const fetchItems = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const params = new URLSearchParams({ page: String(page), size: String(PAGE_SIZE) })
      if (level) params.set('level', level)
      if (type) params.set('type', type)
      if (search) params.set('search', search)
      const data = await adminFetch<{ items: SpeakingQuestion[]; total: number }>('GET', `/speaking?${params}`)
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

  function openEdit(q: SpeakingQuestion) {
    setEditing(q)
    setForm({
      type: q.type,
      title: q.title,
      text: q.text ?? '',
      audio_url: q.audio_url ?? '',
      jlpt_level: q.jlpt_level,
    })
    setModalOpen(true)
  }

  async function handleSave() {
    setError('')
    try {
      const body = { ...form }
      if (editing) {
        await adminFetch('PUT', `/speaking/${editing.id}`, body)
      } else {
        await adminFetch('POST', '/speaking', body)
      }
      setModalOpen(false)
      fetchItems()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed')
    }
  }

  async function handleDelete(id: number) {
    if (!confirm('Delete this speaking question?')) return
    setError('')
    try {
      await adminFetch('DELETE', `/speaking/${id}`)
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
      if (ttsConfig.provider) {
        fd.append('tts_provider', ttsConfig.provider)
        fd.append('tts_url', ttsConfig.tts_url)
        fd.append('tts_model', ttsConfig.tts_model)
        fd.append('voice', ttsConfig.voice)
        fd.append('instructions', ttsConfig.instructions)
        fd.append('sbv_url', ttsConfig.sbv_url)
        fd.append('sbv_model', ttsConfig.sbv_model)
        fd.append('sbv_speaker', ttsConfig.sbv_speaker)
        fd.append('sbv_style', ttsConfig.sbv_style)
      }
      const data = await adminFetch<{ inserted: number }>('POST', '/import/speaking', fd)
      alert(`Imported ${data.inserted} speaking questions${ttsConfig.provider ? ' (audio generation started)' : ''}`)
      fetchItems()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed')
    }
    e.target.value = ''
  }

  const columns = [
    { key: 'id', label: 'ID' },
    { key: 'type', label: 'Type' },
    { key: 'title', label: 'Title' },
    { key: 'text', label: 'Text' },
    { key: 'jlpt_level', label: 'Level' },
  ]

  return (
    <div className={styles.page}>
      <h2 className={styles.title}>Speaking</h2>

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
        <label style={{ display: 'flex', alignItems: 'center', gap: '4px', fontSize: '12px', color: '#64748b', cursor: 'pointer', whiteSpace: 'nowrap' }}>
          <input
            type="checkbox"
            checked={showTTSSettings}
            onChange={(e) => {
              setShowTTSSettings(e.target.checked)
              if (!e.target.checked) setTTSConfig(defaultTTSConfig)
            }}
          />
          Audio
        </label>
      </div>

      {showTTSSettings && (
        <TTSConfigFields config={ttsConfig} onChange={setTTSConfig} />
      )}

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
                    <AudioRegenButton
                      text={q.text}
                      audioUrl={q.audio_url?.startsWith('http') ? q.audio_url : undefined}
                      module="example"
                      onRegenerated={() => fetchItems()}
                    />
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

      <Modal open={modalOpen} title={editing ? 'Edit Speaking Question' : 'Add Speaking Question'} onClose={() => setModalOpen(false)}>
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
            Title
            <input value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} />
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
            Text
            <textarea value={form.text} onChange={(e) => setForm({ ...form, text: e.target.value })} rows={4} />
          </label>
          <label>
            Audio URL
            <input value={form.audio_url} onChange={(e) => setForm({ ...form, audio_url: e.target.value })} placeholder="https://..." />
          </label>
          <button className={styles.saveBtn} onClick={handleSave}>
            Save
          </button>
        </div>
      </Modal>
    </div>
  )
}
