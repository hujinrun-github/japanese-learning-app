import { useState, useRef, useEffect } from 'react'
import Modal from '@/components/Modal/Modal'
import { TTSConfigFields, defaultTTSConfig, getDefaultTTSConfig, type TTSConfig } from '@/components/TTSConfigFields/TTSConfigFields'
import { sha256Hex, exampleAudioUrl } from '@/util/audioHash'
import styles from './AudioRegenButton.module.css'

interface Props {
  text: string
  audioUrl?: string
  module: 'word' | 'example'
  wordId?: number
  onRegenerated?: (newAudioUrl: string) => void
}

function speakWithBrowserTTS(text: string): Promise<boolean> {
  return new Promise((resolve) => {
    const synth = window.speechSynthesis
    if (!synth) {
      resolve(false)
      return
    }
    synth.cancel()
    const u = new SpeechSynthesisUtterance(text)
    u.lang = 'ja-JP'
    u.rate = 0.85
    u.volume = 1
    // Try to pick a Japanese voice
    const voices = synth.getVoices()
    const jaVoice = voices.find(v => v.lang.startsWith('ja'))
    if (jaVoice) u.voice = jaVoice
    u.onend = () => resolve(true)
    u.onerror = () => resolve(false)
    synth.speak(u)
  })
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
  const defaultConfigRef = useRef<TTSConfig>(defaultTTSConfig)

  useEffect(() => { getDefaultTTSConfig().then(c => {
    defaultConfigRef.current = c
    setTTSConfig(c)
  }) }, [])

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

  async function play() {
    // Stop current playback (both audio element and browser TTS)
    if (audioRef.current) {
      audioRef.current.pause()
      audioRef.current = null
    }
    window.speechSynthesis?.cancel()

    if (playing) {
      setPlaying(false)
      return
    }

    // If no text, nothing to play
    if (!text) return

    setPlayError(false)
    let url = regenResult || audioUrl || computedUrl

    if (url) {
      // Try pre-generated audio file first (cache-bust to avoid stale browser cache)
      const audio = new Audio(url + '?t=' + Date.now())
      audio.volume = 1
      audio.onended = () => setPlaying(false)
      audio.onerror = async () => {
        // Fallback to browser TTS
        audioRef.current = null
        const ok = await speakWithBrowserTTS(text)
        if (!ok) {
          setPlayError(true)
          if (errorTimerRef.current) clearTimeout(errorTimerRef.current)
          errorTimerRef.current = setTimeout(() => setPlayError(false), 2000)
        }
        setPlaying(false)
      }
      audio.play().catch(async () => {
        // Fallback to browser TTS
        audioRef.current = null
        const ok = await speakWithBrowserTTS(text)
        if (!ok) {
          setPlayError(true)
          if (errorTimerRef.current) clearTimeout(errorTimerRef.current)
          errorTimerRef.current = setTimeout(() => setPlayError(false), 2000)
        }
        setPlaying(false)
      })
      audioRef.current = audio
      setPlaying(true)
    } else {
      // No URL computed — use browser TTS directly
      setPlaying(true)
      const ok = await speakWithBrowserTTS(text)
      if (!ok) {
        setPlayError(true)
        if (errorTimerRef.current) clearTimeout(errorTimerRef.current)
        errorTimerRef.current = setTimeout(() => setPlayError(false), 2000)
      }
      setPlaying(false)
    }
  }

  function openModal() {
    setTTSConfig({ ...defaultConfigRef.current, provider: 'vllm' })
    setRegenError('')
    setRegenResult(null)
    setModalOpen(true)
  }

  async function handleRegenerate() {
    setRegenerating(true)
    setRegenError('')
    try {
      const token = sessionStorage.getItem('admin_token') || ''
      console.log('[AudioRegen] Starting regen:', { text, module, wordId, provider: ttsConfig.provider })
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
      console.log('[AudioRegen] Response status:', res.status)
      const data = await res.json()
      console.log('[AudioRegen] Response data:', data)
      if (!res.ok) throw new Error(data.error || 'Regenerate failed')
      const newUrl = data.audio_url
      console.log('[AudioRegen] New audio URL:', newUrl)
      setRegenResult(newUrl)
      setPlayError(false)
      onRegenerated?.(data.filename)
      // Auto-play the new audio
      setTimeout(() => {
        console.log('[AudioRegen] Auto-playing:', newUrl)
        const audio = new Audio(newUrl + '?t=' + Date.now())
        audio.onerror = (e) => console.error('[AudioRegen] Playback error:', e)
        audio.onloadeddata = () => console.log('[AudioRegen] Audio loaded')
        audio.play().then(() => console.log('[AudioRegen] Playing')).catch(e => console.error('[AudioRegen] Play failed:', e))
      }, 400)
    } catch (err) {
      console.error('[AudioRegen] Error:', err)
      setRegenError(err instanceof Error ? err.message : 'Regenerate failed')
    } finally {
      setRegenerating(false)
    }
  }

  const effectiveUrl = regenResult || audioUrl || computedUrl
  // Determine audio quality: DB audio > regenerated > browser TTS fallback
  const hasDB = !!audioUrl
  const hasRegen = !!regenResult && !hasDB // regenerated but DB not yet refreshed
  const audioClass = hasDB ? styles.hasAudio : hasRegen ? styles.regenOk : ''
  const playIcon = hasDB ? '🔔' : '🔊'
  const playTitle = !text ? 'No text to speak'
    : playing ? 'Stop'
    : playError ? 'Playback failed'
    : hasDB ? 'Play (HQ audio)'
    : hasRegen ? 'Play (regenerated)'
    : 'Play (browser TTS)'

  return (
    <>
      <button
        className={`${styles.btn} ${audioClass} ${playing ? styles.playing : ''} ${playError ? styles.error : ''}`}
        onClick={play}
        disabled={!text}
        title={playTitle}
      >
        {playError ? '⚠️' : playing ? '⏹' : playIcon}
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
