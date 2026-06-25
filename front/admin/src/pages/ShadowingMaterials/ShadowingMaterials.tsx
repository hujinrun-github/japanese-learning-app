import { useCallback, useEffect, useState } from 'react'
import { adminFetch } from '@/api/client'
import styles from './ShadowingMaterials.module.css'

interface ShadowingMaterialLesson {
  id: number
  title: string
  jlpt_level: string
  shadowing_enabled: boolean
  shadowing_version: number
  video_object_id: number | null
  video_url: string
}

interface ShadowingMaterialDraftResponse {
  lesson: Record<string, unknown>
}

const PAGE_SIZE = 20

export default function ShadowingMaterialsPage() {
  const [items, setItems] = useState<ShadowingMaterialLesson[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [lessonId, setLessonId] = useState('')
  const [videoObjectId, setVideoObjectId] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [drafting, setDrafting] = useState(false)
  const [draftTitle, setDraftTitle] = useState('')
  const [draftLevel, setDraftLevel] = useState('N5')
  const [draftAudioUrl, setDraftAudioUrl] = useState('')
  const [draftVideoUrl, setDraftVideoUrl] = useState('')
  const [draftDurationMs, setDraftDurationMs] = useState('')
  const [draftTranscript, setDraftTranscript] = useState('')
  const [draftJSON, setDraftJSON] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const totalPages = Math.ceil(total / PAGE_SIZE)

  const fetchItems = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const params = new URLSearchParams({ page: String(page), size: String(PAGE_SIZE) })
      if (search.trim()) params.set('search', search.trim())
      const data = await adminFetch<{ items: ShadowingMaterialLesson[]; total: number }>('GET', `/shadowing/materials/lessons?${params}`)
      setItems(data.items || [])
      setTotal(data.total || 0)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load shadowing materials')
    } finally {
      setLoading(false)
    }
  }, [page, search])

  useEffect(() => { fetchItems() }, [fetchItems])

  async function uploadVideo() {
    if (!file) {
      setError('Select a video file first')
      return
    }
    setSaving(true)
    setError('')
    setNotice('')
    try {
      const fd = new FormData()
      fd.append('file', file)
      if (lessonId.trim()) fd.append('lesson_id', lessonId.trim())
      const data = await adminFetch<{ video_object_id: number; video_url: string }>('POST', '/shadowing/materials/videos', fd)
      setNotice(`Video uploaded: #${data.video_object_id} ${data.video_url}`)
      setVideoObjectId(String(data.video_object_id))
      setDraftVideoUrl(data.video_url)
      setFile(null)
      await fetchItems()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Upload failed')
    } finally {
      setSaving(false)
    }
  }

  async function bindVideo(targetLessonID = Number(lessonId), targetVideoObjectID = Number(videoObjectId)) {
    if (!Number.isInteger(targetLessonID) || targetLessonID <= 0 || !Number.isInteger(targetVideoObjectID) || targetVideoObjectID <= 0) {
      setError('Both lesson_id and video_object_id are required')
      return
    }
    setSaving(true)
    setError('')
    setNotice('')
    try {
      const data = await adminFetch<{ video_url: string }>('POST', `/shadowing/materials/lessons/${targetLessonID}/video`, { video_object_id: targetVideoObjectID })
      setNotice(`Lesson #${targetLessonID} bound to ${data.video_url}`)
      await fetchItems()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Bind failed')
    } finally {
      setSaving(false)
    }
  }

  async function generateDraft() {
    const duration = Number(draftDurationMs)
    const videoURL = draftVideoUrl.trim() || (videoObjectId.trim() ? `/api/v1/videos/${videoObjectId.trim()}/stream` : '')
    if (!draftTitle.trim() || !draftLevel.trim() || !Number.isFinite(duration) || duration <= 0 || !draftTranscript.trim()) {
      setError('Title, level, media duration, and transcript are required')
      return
    }
    if (!videoURL && !draftAudioUrl.trim()) {
      setError('Provide either a video URL or an audio URL')
      return
    }

    setDrafting(true)
    setError('')
    setNotice('')
    try {
      const data = await adminFetch<ShadowingMaterialDraftResponse>('POST', '/shadowing/materials/drafts', {
        title: draftTitle.trim(),
        jlpt_level: draftLevel.trim(),
        audio_url: draftAudioUrl.trim(),
        video_url: videoURL,
        media_duration_ms: Math.round(duration),
        transcript: draftTranscript.trim(),
      })
      setDraftJSON(JSON.stringify(data.lesson, null, 2))
      setNotice('Draft material pack generated. Review the JSON before importing it.')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Draft generation failed')
    } finally {
      setDrafting(false)
    }
  }

  function handleSearch() {
    if (page === 1) {
      void fetchItems()
      return
    }
    setPage(1)
  }

  return (
    <div className="adm-page">
      <div className="adm-header">
        <div className="adm-titleRow">
          <h2 className={`adm-title ${styles.title}`}>Shadowing Materials</h2>
          <span className="adm-count">{total} lessons</span>
        </div>
      </div>

      <section className={styles.panel}>
        <h3 className={styles.panelTitle}>Upload or bind video material</h3>
        <div className={styles.formRow}>
          <input type="number" min={1} placeholder="lesson_id" value={lessonId} onChange={e => setLessonId(e.target.value)} />
          <input type="number" min={1} placeholder="video_object_id" value={videoObjectId} onChange={e => setVideoObjectId(e.target.value)} />
          <input type="file" accept="video/*" onChange={e => setFile(e.target.files?.[0] ?? null)} />
          <button className="adm-btn" onClick={uploadVideo} disabled={saving || !file}>{saving ? 'Saving...' : 'Upload video'}</button>
          <button className="adm-btnOutline" onClick={() => bindVideo()} disabled={saving}>Bind existing object</button>
        </div>
        <p className={styles.hint}>Upload stores the binary in MinIO, writes metadata to Postgres video_objects, and binds it when lesson_id is provided.</p>
      </section>

      <section className={styles.panel}>
        <h3 className={styles.panelTitle}>Generate draft material pack</h3>
        <div className={styles.draftGrid}>
          <input placeholder="Lesson title" value={draftTitle} onChange={e => setDraftTitle(e.target.value)} />
          <input placeholder="JLPT level" value={draftLevel} onChange={e => setDraftLevel(e.target.value)} />
          <input placeholder="Audio URL (optional)" value={draftAudioUrl} onChange={e => setDraftAudioUrl(e.target.value)} />
          <input placeholder="Video URL" value={draftVideoUrl} onChange={e => setDraftVideoUrl(e.target.value)} />
          <input type="number" min={1} placeholder="Media duration ms" value={draftDurationMs} onChange={e => setDraftDurationMs(e.target.value)} />
        </div>
        <textarea
          className={styles.draftTranscript}
          placeholder="Paste reviewed transcript text here. The draft generator splits by Japanese sentence punctuation and allocates timing evenly."
          value={draftTranscript}
          onChange={e => setDraftTranscript(e.target.value)}
        />
        <div className={styles.draftActions}>
          <button className="adm-btn" onClick={generateDraft} disabled={drafting}>{drafting ? 'Generating...' : 'Generate draft JSON'}</button>
        </div>
        {draftJSON && <textarea className={styles.draftOutput} readOnly value={draftJSON} />}
        <p className={styles.hint}>This is an audit draft, not an automatic import. ASR, translation, and furigana generation remain a separate future automation step.</p>
      </section>

      <div className="adm-toolbar">
        <input placeholder="Search lesson title..." value={search} onChange={e => setSearch(e.target.value)} onKeyDown={e => e.key === 'Enter' && handleSearch()} />
        <button className="adm-btn" onClick={handleSearch}>Search</button>
      </div>

      {error && <p className="adm-error">{error}</p>}
      {notice && <p className="adm-success">{notice}</p>}

      {loading ? <p className="adm-loading">Loading shadowing lessons...</p> : (
        <>
          <table className="adm-table">
            <thead>
              <tr>
                <th>ID</th>
                <th>Title</th>
                <th>Level</th>
                <th>Enabled</th>
                <th>Version</th>
                <th>Video Object</th>
                <th>Video URL</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {items.length === 0 ? <tr><td colSpan={8} className="adm-empty">No lessons found</td></tr> :
                items.map(item => (
                  <tr key={item.id}>
                    <td className="adm-id">{item.id}</td>
                    <td className="adm-kanji">{item.title}</td>
                    <td><span className="adm-levelBadge">{item.jlpt_level}</span></td>
                    <td>{item.shadowing_enabled ? 'Yes' : 'No'}</td>
                    <td>{item.shadowing_version}</td>
                    <td>{item.video_object_id ?? '-'}</td>
                    <td className={styles.urlCell} title={item.video_url}>{item.video_url || '-'}</td>
                    <td className="adm-actions">
                      <button onClick={() => {
                        setLessonId(String(item.id))
                        setDraftTitle(item.title)
                        setDraftLevel(item.jlpt_level)
                        if (item.video_object_id) setVideoObjectId(String(item.video_object_id))
                        if (item.video_url) setDraftVideoUrl(item.video_url)
                      }}>Use as target</button>
                      {item.video_object_id && <button onClick={() => bindVideo(item.id, item.video_object_id!)} disabled={saving}>Rebind</button>}
                    </td>
                  </tr>
                ))
              }
            </tbody>
          </table>
          <div className="adm-pagination">
            <button disabled={page <= 1} onClick={() => setPage(p => p - 1)}>Prev</button>
            <span className="adm-pageInfo">Page <strong>{page}</strong> / <strong>{totalPages || 1}</strong></span>
            <button disabled={page >= totalPages} onClick={() => setPage(p => p + 1)}>Next</button>
          </div>
        </>
      )}
    </div>
  )
}
