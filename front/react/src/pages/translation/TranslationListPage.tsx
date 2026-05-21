import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiFetch } from '@/api/client'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import type { TranslationSource } from '@/types/api'
import styles from './TranslationListPage.module.css'

export function TranslationListPage() {
  const navigate = useNavigate()
  const [sources, setSources] = useState<TranslationSource[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [showImport, setShowImport] = useState(false)
  const [importTab, setImportTab] = useState<'paste' | 'url'>('paste')
  const [pasteContent, setPasteContent] = useState('')
  const [pasteTitle, setPasteTitle] = useState('')
  const [importURL, setImportURL] = useState('')
  const [importing, setImporting] = useState(false)

  useEffect(() => { fetchSources() }, [])

  async function fetchSources() {
    setLoading(true)
    setError('')
    try {
      const data = await apiFetch<TranslationSource[]>('GET', '/api/v1/translation/sources')
      setSources(data ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load sources')
    } finally {
      setLoading(false)
    }
  }

  async function handlePasteImport() {
    if (!pasteContent.trim()) return
    setImporting(true)
    try {
      await apiFetch('POST', '/api/v1/translation/sources', {
        title: pasteTitle || 'Manual Import',
        source_type: 'manual',
        content: pasteContent,
      })
      setPasteContent('')
      setPasteTitle('')
      setShowImport(false)
      fetchSources()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed')
    } finally {
      setImporting(false)
    }
  }

  async function handleURLImport() {
    if (!importURL.trim()) return
    setImporting(true)
    try {
      await apiFetch('POST', '/api/v1/translation/sources/import', { url: importURL })
      setImportURL('')
      setShowImport(false)
      fetchSources()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed')
    } finally {
      setImporting(false)
    }
  }

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>翻訳練習</h1>

      <div className={styles.cardRow}>
        <button className={styles.mainCard} onClick={() => navigate('/translation/practice?mode=daily')}>
          <span className={styles.cardIcon}>📅</span>
          <span className={styles.cardLabel}>今日任务</span>
        </button>
        <button className={styles.mainCard} onClick={() => navigate('/translation/practice?mode=free')}>
          <span className={styles.cardIcon}>📝</span>
          <span className={styles.cardLabel}>自由练习</span>
        </button>
      </div>

      {error && <p className={styles.error}>{error}</p>}

      {loading ? (
        <div className={styles.center}><Spinner size="lg" /></div>
      ) : sources.length === 0 ? (
        <EmptyState icon="📖" title="暂无翻译素材" description="点击下方按钮导入" />
      ) : (
        <div className={styles.sourceList}>
          {sources.map(s => (
            <div
              key={s.id}
              className={styles.sourceCard}
              onClick={() => navigate(`/translation/practice?mode=free&source_id=${s.id}`)}
            >
              <div className={styles.sourceHeader}>
                <span className={styles.sourceTitle}>{s.title}</span>
                <span className={styles.sourceType}>{s.source_type}</span>
              </div>
              <p className={styles.sourcePreview}>{s.raw_content.slice(0, 100)}…</p>
            </div>
          ))}
        </div>
      )}

      <button className={styles.importBtn} onClick={() => setShowImport(!showImport)}>
        + 导入新素材
      </button>

      {showImport && (
        <div className={styles.importDialog}>
          <div className={styles.importTabs}>
            <button
              className={`${styles.importTab} ${importTab === 'paste' ? styles.importTabActive : ''}`}
              onClick={() => setImportTab('paste')}
            >
              粘贴文本
            </button>
            <button
              className={`${styles.importTab} ${importTab === 'url' ? styles.importTabActive : ''}`}
              onClick={() => setImportTab('url')}
            >
              输入 URL
            </button>
          </div>

          {importTab === 'paste' ? (
            <div className={styles.importForm}>
              <input
                className={styles.importInput}
                placeholder="素材标题（可选）"
                value={pasteTitle}
                onChange={e => setPasteTitle(e.target.value)}
              />
              <textarea
                className={styles.importTextarea}
                placeholder="粘贴日语或中文文本…"
                value={pasteContent}
                onChange={e => setPasteContent(e.target.value)}
                rows={8}
              />
              <button className={styles.submitBtn} onClick={handlePasteImport} disabled={importing || !pasteContent.trim()}>
                {importing ? <Spinner size="sm" /> : '导入'}
              </button>
            </div>
          ) : (
            <div className={styles.importForm}>
              <input
                className={styles.importInput}
                placeholder="https://…"
                value={importURL}
                onChange={e => setImportURL(e.target.value)}
              />
              <button className={styles.submitBtn} onClick={handleURLImport} disabled={importing || !importURL.trim()}>
                {importing ? <Spinner size="sm" /> : '抓取并导入'}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
