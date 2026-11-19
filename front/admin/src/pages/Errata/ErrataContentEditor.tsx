import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { adminFetch } from '@/api/client'
import {
  defaultTTSConfig,
  getDefaultTTSConfig,
  TTSConfigFields,
  type TTSConfig,
} from '@/components/TTSConfigFields/TTSConfigFields'
import {
  cacheBustAudioURL,
  changedContentValues,
  contentDraft,
  invalidJSONField,
  resolutionBlockedReason,
  type ErrataContent,
} from './errataCorrectionRules'
import styles from './Errata.module.css'

type Props = {
  report: { id: number; category: string; description: string }
  onClose: () => void
  onResolved: () => void
}

type GeneratedAudio = { url: string; verified: boolean; text: string; sourceText: string; version: number }

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback
}

export default function ErrataContentEditor({ report, onClose, onResolved }: Props) {
  const dialog = useRef<HTMLDialogElement>(null)
  const titleID = useId()
  const [content, setContent] = useState<ErrataContent | null>(null)
  const [draft, setDraft] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState<'save' | 'audio' | 'resolve' | null>(null)
  const [error, setError] = useState('')
  const [playbackError, setPlaybackError] = useState('')
  const [notice, setNotice] = useState('')
  const [attempt, setAttempt] = useState(0)
  const [targetKey, setTargetKey] = useState('')
  const [synthesisText, setSynthesisText] = useState('')
  const [savedSynthesisText, setSavedSynthesisText] = useState('')
  const [generated, setGenerated] = useState<Record<string, GeneratedAudio>>({})
  const [listenedTarget, setListenedTarget] = useState('')
  const [confirmed, setConfirmed] = useState(false)
  const [ttsConfig, setTTSConfig] = useState<TTSConfig>({ ...defaultTTSConfig, provider: 'vllm' })
  const [playbackVersion, setPlaybackVersion] = useState(Date.now)
  const generatedVersions = useRef<Record<string, number>>({})
  const selectedTarget = useRef('')

  useEffect(() => {
    dialog.current?.showModal()
  }, [])

  useEffect(() => {
    let ignore = false
    setLoading(true)
    setError('')
    async function loadContent() {
      try {
        const next = await adminFetch<ErrataContent>('GET', `/errata/${report.id}/content`)
        if (ignore) return
        setContent(next)
        setDraft(contentDraft(next.fields))
        const target = next.audio_targets?.length === 1 ? next.audio_targets[0] : undefined
        setTargetKey(target?.key ?? '')
        selectedTarget.current = target?.key ?? ''
        setSynthesisText(target?.text ?? '')
        setSavedSynthesisText(target?.text ?? '')
      } catch (err) {
        if (!ignore) setError(errorMessage(err, '加载原内容失败'))
      } finally {
        if (!ignore) setLoading(false)
      }
    }
    void loadContent()
    return () => { ignore = true }
  }, [report.id, attempt])

  useEffect(() => {
    let ignore = false
    getDefaultTTSConfig().then(config => {
      if (!ignore) setTTSConfig({
        ...config,
        provider: 'vllm',
        instructions: '自然な日本語で、入力された文章だけを正確に読み上げてください。',
      })
    }).catch(err => {
      console.error('加载勘误音频配置失败', err)
    })
    return () => { ignore = true }
  }, [])

  const changedValues = changedContentValues(content?.fields ?? [], draft)
  const dirty = Object.keys(changedValues).length > 0
  const jsonError = invalidJSONField(content?.fields ?? [], draft)
  const target = content?.audio_targets?.find(item => item.key === targetKey)
  const generatedAudio = generated[targetKey]
  const audioDirty = Boolean(target) && synthesisText !== savedSynthesisText
  const unheardGenerated = Object.entries(generated).filter(([, audio]) => !audio.verified)
  const requiresAudio = (report.category === 'audio' && Boolean(content?.audio_targets?.length))
    || Object.keys(generated).length > 0
  const audioVerified = Boolean(target && listenedTarget === target.key) && unheardGenerated.length === 0
  const resolutionState = {
    loaded: Boolean(content),
    busy: loading || busy !== null,
    dirty,
    audioDirty,
    error: error || playbackError,
    audioRequired: requiresAudio,
    audioVerified,
    confirmed,
  }
  const confirmationBlocked = resolutionBlockedReason({ ...resolutionState, confirmed: true })
  const resolutionBlocked = resolutionBlockedReason(resolutionState)

  function resetVerification() {
    setConfirmed(false)
    setListenedTarget('')
  }

  function acceptContent(next: ErrataContent) {
    setContent(next)
    setDraft(contentDraft(next.fields))
    setGenerated(previous => Object.fromEntries(Object.entries(previous).filter(([key, audio]) =>
      next.audio_targets?.some(item => item.key === key && item.text === audio.sourceText),
    )))
    for (const [key, audio] of Object.entries(generated)) {
      if (!next.audio_targets?.some(item => item.key === key && item.text === audio.sourceText)) {
        delete generatedVersions.current[key]
      }
    }
    setPlaybackVersion(Date.now())
    resetVerification()
  }

  function close() {
    if (busy) return
    if ((dirty || audioDirty) && !window.confirm('还有未保存的修改，确认放弃并关闭？')) return
    onClose()
  }

  function reloadContent() {
    if (busy) return
    if ((dirty || audioDirty) && !window.confirm('重新加载会放弃尚未保存的修改，确认继续？')) return
    setContent(null)
    setDraft({})
    setGenerated({})
    generatedVersions.current = {}
    selectedTarget.current = ''
    setTargetKey('')
    setSynthesisText('')
    setSavedSynthesisText('')
    setNotice('')
    setError('')
    setPlaybackError('')
    resetVerification()
    setLoading(true)
    setAttempt(value => value + 1)
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!content || !dirty || busy || jsonError) return
    setBusy('save')
    setError('')
    setNotice('')
    resetVerification()
    try {
      const next = await adminFetch<ErrataContent>('PUT', `/errata/${report.id}/content`, {
        revision: content.revision,
        values: changedValues,
      })
      acceptContent(next)
      const nextTarget = next.audio_targets?.find(item => item.key === targetKey)
      if (nextTarget?.text !== target?.text) {
        setSynthesisText(nextTarget?.text ?? '')
        setSavedSynthesisText(nextTarget?.text ?? '')
      }
      setNotice('原内容已保存，报告仍待处理。请检查修改结果后确认修复。')
    } catch (err) {
      setError(errorMessage(err, '保存原内容失败'))
    } finally {
      setBusy(null)
    }
  }

  function selectAudio(key: string) {
    if (audioDirty && !window.confirm('朗读文本尚未重新生成，确认切换音频并放弃这次文本修改？')) return
    const next = content?.audio_targets?.find(item => item.key === key)
    setTargetKey(key)
    selectedTarget.current = key
    const text = generated[key]?.text ?? next?.text ?? ''
    setSynthesisText(text)
    setSavedSynthesisText(text)
    resetVerification()
    setNotice('')
  }

  async function regenerate() {
    if (!content || !target || !synthesisText.trim() || busy || dirty || !ttsConfig.provider) return
    const key = target.key
    const text = synthesisText.trim()
    setBusy('audio')
    setError('')
    setPlaybackError('')
    setNotice('')
    resetVerification()
    generatedVersions.current[key] = -1
    let audioSaved = false
    try {
      const result = await adminFetch<{ audio_url: string; content?: ErrataContent }>('POST', `/errata/${report.id}/audio`, {
        ...ttsConfig,
        revision: content.revision,
        target_key: key,
        text,
      })
      audioSaved = true
      const next = result.content ?? await adminFetch<ErrataContent>('GET', `/errata/${report.id}/content`)
      acceptContent(next)
      const version = Date.now()
      generatedVersions.current[key] = version
      setGenerated(previous => ({ ...previous, [key]: { url: result.audio_url, text, sourceText: target.text, version, verified: false } }))
      setSynthesisText(text)
      setSavedSynthesisText(text)
      setNotice('新音频已保存。请完整试听，确认读音正确后再标记已处理。')
    } catch (err) {
      setError(audioSaved ? '音频已保存，但刷新原内容失败，请重新打开反馈后检查。' : errorMessage(err, '重新生成音频失败'))
    } finally {
      setBusy(null)
    }
  }

  function verifyPlayback(key: string, generationVersion?: number) {
    if (selectedTarget.current !== key) return
    if (generationVersion !== undefined && generatedVersions.current[key] !== generationVersion) return
    if (generationVersion === undefined && generatedVersions.current[key] !== undefined) return
    setListenedTarget(key)
    setConfirmed(false)
    if (generationVersion !== undefined) {
      setGenerated(previous => {
        const audio = previous[key]
        if (!audio || audio.version !== generationVersion) return previous
        return { ...previous, [key]: { ...audio, verified: true } }
      })
    }
    setPlaybackError('')
  }

  function playbackFailed() {
    resetVerification()
    setPlaybackError('音频加载或播放失败，请检查音频地址或重新生成后再试听。')
  }

  async function resolve() {
    if (resolutionBlocked) return
    setBusy('resolve')
    setError('')
    try {
      await adminFetch('PATCH', `/errata/${report.id}`, { status: 'resolved', confirmed: true, revision: content?.revision })
      onResolved()
    } catch (err) {
      setError(errorMessage(err, '标记已处理失败'))
    } finally {
      setBusy(null)
    }
  }

  return (
    <dialog
      ref={dialog}
      className={styles.editorDialog}
      aria-labelledby={titleID}
      onCancel={event => { event.preventDefault(); close() }}
    >
      <div className={styles.editorHeader}>
        <div>
          <h2 id={titleID}>修正原内容</h2>
          <p>报告 #{report.id}{content ? ` · ${content.title}` : ''}</p>
        </div>
        <button type="button" className="adm-btnOutline" onClick={close} disabled={busy !== null} aria-label="关闭修正原内容">关闭</button>
      </div>
      <div className={styles.editorBody}>
        <p className={styles.feedbackDescription}><strong>问题反馈：</strong>{report.description}</p>
        {error && <p className="adm-error" role="alert">{error}</p>}
        {playbackError && <p className="adm-error" role="alert">{playbackError}</p>}
        {(error || playbackError) && content && <button type="button" className="adm-btnOutline" disabled={busy !== null} onClick={reloadContent}>重新加载原内容</button>}
        {notice && <p className={styles.notice} role="status">{notice}</p>}
        {loading ? (
          <p role="status">正在加载原内容…</p>
        ) : content ? (
          <>
            <form onSubmit={save}>
              <h3>内容修正</h3>
              <p className={styles.hint}>保存会更新学习内容。报告将在你确认修复后标记为已处理。</p>
              <div className={styles.fields}>
                {content.fields.map(field => (
                  <div key={field.key} className={styles.field}>
                    <label htmlFor={`${titleID}-${field.key}`} className={styles.fieldTitle}>
                      {field.label}{field.readonly ? '（只读）' : ''}
                    </label>
                    <div className={styles.comparison}>
                      <div>
                        <span className={styles.columnLabel}>当前保存的内容</span>
                        <pre>{field.value || '（空）'}</pre>
                        {field.value && field.key.endsWith('.audio_url') && (
                          <audio className={styles.savedMedia} aria-label={`${field.label}试听`} controls preload="none" src={cacheBustAudioURL(field.value, playbackVersion)} />
                        )}
                        {field.value && field.key.endsWith('.video_url') && (
                          <video className={styles.savedMedia} aria-label={`${field.label}预览`} controls preload="none" src={cacheBustAudioURL(field.value, playbackVersion)} />
                        )}
                      </div>
                      {field.readonly ? (
                        <p className={styles.hint}>{field.key.endsWith('.audio_url') || field.key.endsWith('.video_url')
                          ? '媒体包含时间轴，请在素材管理中修复并检查同步。'
                          : '此内容关联已有练习记录，保留原文。可修正参考答案等其他字段。'}</p>
                      ) : (
                        <div>
                          <span className={styles.columnLabel}>修改后</span>
                          {field.kind === 'textarea' || field.kind === 'json' || field.value.length > 100 ? (
                            <textarea
                              id={`${titleID}-${field.key}`}
                              aria-label={`${field.label}（修改后）`}
                              rows={field.kind === 'json' ? 5 : 3}
                              value={draft[field.key] ?? field.value}
                              disabled={busy !== null}
                              spellCheck={false}
                              onChange={event => {
                                setDraft(previous => ({ ...previous, [field.key]: event.target.value }))
                                resetVerification()
                              }}
                            />
                          ) : (
                            <input
                              id={`${titleID}-${field.key}`}
                              aria-label={`${field.label}（修改后）`}
                              value={draft[field.key] ?? field.value}
                              disabled={busy !== null}
                              onChange={event => {
                                setDraft(previous => ({ ...previous, [field.key]: event.target.value }))
                                resetVerification()
                              }}
                            />
                          )}
                        </div>
                      )}
                    </div>
                  </div>
                ))}
              </div>
              {jsonError && <p className="adm-error" role="alert">{jsonError} 的 JSON 格式不正确，请修正后保存。</p>}
              <div className={styles.editorActions}>
                <button type="submit" className="adm-btn" disabled={!dirty || busy !== null || Boolean(jsonError)}>
                  {busy === 'save' ? '正在保存…' : '保存原内容'}
                </button>
                <span className={styles.hint}>{dirty ? `有 ${Object.keys(changedValues).length} 项未保存修改` : '当前内容已保存'}</span>
              </div>
            </form>

            <details className={styles.audioSection} open={report.category === 'audio' || undefined}>
              <summary>🎧 音频修复与试听</summary>
              {content.audio_targets?.length ? (
                <div className={styles.audioBody}>
                  <label className={styles.control}>
                    需要修复的音频
                    <select value={targetKey} disabled={busy !== null} onChange={event => selectAudio(event.target.value)}>
                      <option value="">请选择单词或具体例句</option>
                      {content.audio_targets.map(item => <option key={item.key} value={item.key}>{item.label}</option>)}
                    </select>
                  </label>
                  {target ? (
                    <>
                      <p className={styles.sourceText}><strong>对应原文：</strong>{target.text}</p>
                      {generatedAudio ? (
                        <div className={styles.audioPlayer}>
                          <strong>生成后的音频</strong>
                          <audio
                            key={`${target.key}-${generatedAudio.version}`}
                            aria-label={`${target.label}生成后的音频`}
                            controls
                            preload="none"
                            src={cacheBustAudioURL(generatedAudio.url, generatedAudio.version)}
                            onEnded={() => verifyPlayback(target.key, generatedAudio.version)}
                            onError={playbackFailed}
                          />
                          <span className={styles.hint}>{generatedAudio.verified ? '已完整试听此段新音频' : '请播放至结束以确认这段音频'}</span>
                        </div>
                      ) : target.audio_url ? (
                        <div className={styles.audioPlayer}>
                          <strong>当前音频</strong>
                          <audio
                            key={`${target.key}-${playbackVersion}`}
                            aria-label={`${target.label}当前音频`}
                            controls
                            preload="none"
                            src={cacheBustAudioURL(target.audio_url, playbackVersion)}
                            onEnded={() => verifyPlayback(target.key)}
                            onError={playbackFailed}
                          />
                        </div>
                      ) : <p className={styles.hint}>当前没有可播放的音频，请重新生成。</p>}
                      <label className={styles.control}>
                        朗读文本（可用假名修正多音字读音）
                        <textarea
                          rows={3}
                          value={synthesisText}
                          disabled={busy !== null}
                          onChange={event => { setSynthesisText(event.target.value); resetVerification() }}
                        />
                      </label>
                      <p className={styles.hint}>朗读文本只用于生成音频，不会替换页面显示的原文。重新生成会替换对应音频。</p>
                      <fieldset className={styles.ttsSettings} disabled={busy !== null}>
                        <legend>语音生成设置</legend>
                        <TTSConfigFields config={ttsConfig} onChange={setTTSConfig} />
                      </fieldset>
                      <button
                        type="button"
                        className="adm-btn"
                        onClick={() => { void regenerate() }}
                        disabled={busy !== null || dirty || !synthesisText.trim() || !ttsConfig.provider}
                      >{busy === 'audio' ? '正在生成音频…' : '🔄 重新生成音频'}</button>
                      {dirty && <p className={styles.hint}>请先保存原内容，再生成对应音频。</p>}
                      {unheardGenerated.length > 0 && (
                        <p className={styles.hint}>还有 {unheardGenerated.length} 段新音频未完整试听，请在上方选择相应音频检查。</p>
                      )}
                    </>
                  ) : <p className={styles.hint}>请根据反馈说明选择具体音频，再检查或重新生成。</p>}
                </div>
              ) : (
                <p className={styles.hint}>此内容暂无可直接生成的音频。带有句子时间轴的媒体需单独修复，以保留音画同步；请检查实际播放效果后再确认已修复。</p>
              )}
            </details>

            <div className={styles.confirmSection}>
              <h3>确认修复</h3>
              <label className={styles.confirmation}>
                <input
                  type="checkbox"
                  checked={confirmed}
                  disabled={Boolean(confirmationBlocked)}
                  onChange={event => setConfirmed(event.target.checked)}
                />
                已检查原内容，确认问题已修复
              </label>
              {confirmationBlocked && <p className={styles.hint}>{confirmationBlocked}</p>}
              <div className={styles.editorActions}>
                <button type="button" className="adm-btnOutline" onClick={close} disabled={busy !== null}>稍后处理</button>
                <button
                  type="button"
                  className="adm-btn"
                  disabled={Boolean(resolutionBlocked)}
                  onClick={() => { void resolve() }}
                >{busy === 'resolve' ? '正在标记…' : '确认修复并标记已处理'}</button>
              </div>
            </div>
          </>
        ) : (
          <button type="button" className="adm-btn" onClick={reloadContent}>重新加载原内容</button>
        )}
      </div>
    </dialog>
  )
}
