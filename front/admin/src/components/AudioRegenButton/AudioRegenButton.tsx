import { useState, useRef, useEffect } from 'react'
import Modal from '@/components/Modal/Modal'
import { TTSConfigFields, defaultTTSConfig, type TTSConfig } from '@/components/TTSConfigFields/TTSConfigFields'
import { sha256Hex, exampleAudioUrl } from '@/util/audioHash'
import styles from './AudioRegenButton.module.css'

interface Props {
  text: string
  audioUrl?: string
  module: 'word' | 'example'
  wordId?: number
  onRegenerated?: (newAudioUrl: string) => void
}

export function AudioRegenButton({ text, audioUrl, module, wordId, onRegenerated }: Props) {
  const [playing, setPlaying] = useState(false)
  const [playError, setPlayError] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [ttsConfig, setTTSConfig] = useState<TTSConfig>(defaultTTSConfig)
  const [regenerating, setRegenerating] = useState(false)
  const [regenError, setRegenError] = useState('')
  const [regenResult, setRegenResult] = useState<string | null>(null)
  const [computedUrl, setComputedUrl] = useState<string>('')
  const audioRef = useRef<HTMLAudioElement | null>(null)
  const errorTimerRef = useRef<ReturnType<typeof setTimeout>>()

  // Compute audio URL from text when no explicit audioUrl is provided
  useEffect(() => {
    if (audioUrl || !text) {
      setComputedUrl('')
      return
    }
    if (module === 'example') {
      sha256Hex(text).then(hash => setComputedUrl(exampleAudioUrl(hash)))
    } else if (module === 'word') {
      sha256Hex(text).then(hash => setComputedUrl(`/audio/words/${hash}.wav`))
    }
  }, [text, audioUrl, module])

  function play() {
    const url = regenResult || audioUrl || computedUrl
    if (!url) return

    // Stop current playback
    if (audioRef.current) {
      audioRef.current.pause()
      audioRef.current = null
    }

    if (playing) {
      setPlaying(false)
      return
    }

    setPlayError(false)
    const audio = new Audio(url)
    audio.volume = 1
    audio.onended = () => setPlaying(false)
    audio.onerror = () => {
      setPlaying(false)
      setPlayError(true)
      if (errorTimerRef.current) clearTimeout(errorTimerRef.current)
      errorTimerRef.current = setTimeout(() => setPlayError(false), 2000)
    }
    audio.play().catch(() => {
      setPlaying(false)
      setPlayError(true)
      if (errorTimerRef.current) clearTimeout(errorTimerRef.current)
      errorTimerRef.current = setTimeout(() => setPlayError(false), 2000)
    })
    audioRef.current = audio
    setPlaying(true)
  }

  function openModal() {
    setTTSConfig({ ...defaultTTSConfig, provider: 'sbv' })
    setRegenError('')
    setRegenResult(null)
    setModalOpen(true)
  }

  async function handleRegenerate() {
    setRegenerating(true)
    setRegenError('')
    try {
      const token = sessionStorage.getItem('admin_token') || ''
      const res = await fetch('/api/admin/audio/regen', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`,
        },
        body: JSON.stringify({
          text,
          module,
          word_id: wordId ?? 0,
          provider: ttsConfig.provider,
          tts_url: ttsConfig.tts_url,
          tts_model: ttsConfig.tts_model,
          voice: ttsConfig.voice,
          instructions: ttsConfig.instructions,
          sbv_url: ttsConfig.sbv_url,
          sbv_model: ttsConfig.sbv_model,
          sbv_speaker: ttsConfig.sbv_speaker,
          sbv_style: ttsConfig.sbv_style,
        }),
      })
      const data = await res.json()
      if (!res.ok) throw new Error(data.error || 'Regenerate failed')
      const newUrl = data.audio_url
      setRegenResult(newUrl)
      setPlayError(false)
      onRegenerated?.(data.filename)
      // Auto-play the new audio
      setTimeout(() => {
        const audio = new Audio(newUrl)
        audio.onerror = () => {}
        audio.play().catch(() => {})
      }, 400)
    } catch (err) {
      setRegenError(err instanceof Error ? err.message : 'Regenerate failed')
    } finally {
      setRegenerating(false)
    }
  }

  const effectiveUrl = regenResult || audioUrl || computedUrl

  return (
    <>
      <button
        className={`${styles.btn} ${playing ? styles.playing : ''} ${playError ? styles.error : ''}`}
        onClick={play}
        disabled={!effectiveUrl}
        title={
          !effectiveUrl ? 'No audio — use 🔄 to generate'
          : playing ? 'Stop'
          : playError ? 'Playback failed — file may not exist'
          : 'Play'
        }
      >
        {playError ? '⚠️' : playing ? '⏹' : '🔊'}
      </button>
      <button
        className={styles.btn}
        onClick={openModal}
        title="Regenerate audio"
      >
        🔄
      </button>

      <Modal open={modalOpen} title="Regenerate Audio" onClose={() => setModalOpen(false)}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
          <div style={{ fontSize: '13px', color: '#475569' }}>
            <strong>Text:</strong> {text}
          </div>

          <TTSConfigFields config={ttsConfig} onChange={setTTSConfig} />

          {regenError && (
            <p style={{ color: '#ef4444', fontSize: '12px', margin: 0 }}>{regenError}</p>
          )}

          {regenResult && (
            <p style={{ color: '#16a34a', fontSize: '12px', margin: 0 }}>
              Generated: {regenResult}
            </p>
          )}

          <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
            <button
              onClick={handleRegenerate}
              disabled={regenerating || !ttsConfig.provider}
              style={{
                padding: '8px 20px',
                background: ttsConfig.provider ? '#2563eb' : '#94a3b8',
                color: '#fff',
                border: 'none',
                borderRadius: '4px',
                cursor: ttsConfig.provider ? 'pointer' : 'not-allowed',
                fontSize: '13px',
              }}
            >
              {regenerating ? 'Generating...' : 'Generate'}
            </button>
          </div>
        </div>
      </Modal>
    </>
  )
}
