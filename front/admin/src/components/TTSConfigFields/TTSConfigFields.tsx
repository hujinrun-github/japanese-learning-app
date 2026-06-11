import { adminFetch } from '../../api/client'

export interface TTSConfig {
  provider: string
  tts_url: string
  tts_model: string
  voice: string
  instructions: string
  sbv_url: string
  sbv_model: string
  sbv_speaker: string
  sbv_style: string
  tts_force: boolean
}

const staticDefaults: TTSConfig = {
  provider: '',
  tts_url: '',
  tts_model: 'Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice',
  voice: 'ono_anna',
  instructions: 'Pronounce only the exact given word, in isolation. No extra sounds, no prefix, no suffix. Clean single-word pronunciation.',
  sbv_url: 'http://127.0.0.1:7862',
  sbv_model: 'amitaro',
  sbv_speaker: 'あみたろ',
  sbv_style: 'Neutral',
  tts_force: false,
}

export const defaultTTSConfig: TTSConfig = { ...staticDefaults }

const GRADIO_URL_DEFAULT = 'http://127.0.0.1:8000'
const GRADIO_SPEAKER_DEFAULT = 'Vivian'
const GRADIO_INSTRUCTIONS_DEFAULT = '標準語で、自然にはっきり発音してください。'

// fetchedDefaults caches the server-provided defaults (e.g. eth0-based tts_url).
let fetchedDefaults: Partial<TTSConfig> | null = null

export async function getDefaultTTSConfig(): Promise<TTSConfig> {
  if (!fetchedDefaults) {
    try {
      const data = await adminFetch<{ tts_url: string }>('GET', '/tts-defaults')
      fetchedDefaults = { tts_url: data.tts_url }
    } catch (err) {
      console.error('Failed to fetch TTS defaults from server:', err)
      // Fallback: use window.location.hostname, so the field is not empty.
      // User can still edit it manually.
      fetchedDefaults = { tts_url: `http://${window.location.hostname}:8091/v1/audio/speech` }
    }
  }
  return { ...staticDefaults, ...fetchedDefaults }
}

// Model/Voice/Speaker/Style 的常见值（作为 datalist 提示，可以自由输入）
const VOICE_SUGGESTIONS = ['aiden', 'dylan', 'eric', 'ono_anna', 'ryan', 'serena', 'sohee', 'uncle_fu', 'vivian']
const MODEL_SUGGESTIONS = ['Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice', 'Qwen/Qwen3-TTS-24Hz-0.6B-CustomVoice']
const SBV_MODEL_SUGGESTIONS = ['amitaro']
const SBV_SPEAKER_SUGGESTIONS = ['あみたろ']
const SBV_STYLE_SUGGESTIONS = ['Neutral']
const GRADIO_SPEAKER_SUGGESTIONS = ['Serena', 'Vivian', 'Uncle Fu', 'Ryan', 'Aiden', 'Ono Anna', 'Sohee', 'Eric', 'Dylan']

// ---- 可复用的输入组件 ----

const fieldLabelStyle: React.CSSProperties = {
  fontSize: '12px', fontWeight: 600, color: '#5c5650',
}
const inputBaseStyle: React.CSSProperties = {
  padding: '9px 12px', fontSize: '13px', border: '1.5px solid #d9d3cb', borderRadius: '6px',
  background: '#fafaf7', color: '#3d3833', outline: 'none', width: '100%', boxSizing: 'border-box',
  transition: 'border-color 0.2s, box-shadow 0.2s',
}

function FieldLabel({ children }: { children: React.ReactNode }) {
  return <span style={fieldLabelStyle}>{children}</span>
}

function SelectField({ label, value, onChange, options, placeholder }: {
  label: string; value: string; onChange: (v: string) => void
  options: { value: string; label: string }[]; placeholder?: string
}) {
  return (
    <label style={{ display: 'flex', flexDirection: 'column', gap: '4px', flex: '1', minWidth: '160px' }}>
      <FieldLabel>{label}</FieldLabel>
      <select value={value} onChange={(e) => onChange(e.target.value)} style={{ ...inputBaseStyle, cursor: 'pointer' }}>
        {placeholder && <option value="">{placeholder}</option>}
        {options.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
      </select>
    </label>
  )
}

function TextField({ label, value, onChange, placeholder, type, datalist }: {
  label: string; value: string; onChange: (v: string) => void
  placeholder?: string; type?: string; datalist?: string[]
}) {
  const listId = label.replace(/\s+/g, '_').toLowerCase()
  return (
    <label style={{ display: 'flex', flexDirection: 'column', gap: '4px', flex: '1', minWidth: '180px' }}>
      <FieldLabel>{label}</FieldLabel>
      <input
        type={type || 'text'} value={value} placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)} list={datalist ? listId : undefined}
        style={inputBaseStyle}
      />
      {datalist && <datalist id={listId}>{datalist.map((s) => <option key={s} value={s} />)}</datalist>}
    </label>
  )
}

// ---- 主组件 ----

interface Props {
  config: TTSConfig
  onChange: (c: TTSConfig) => void
  showForce?: boolean
}

export function TTSConfigFields({ config, onChange, showForce }: Props) {
  const isVLLM = config.provider === 'vllm'
  const isSBV = config.provider === 'sbv'
  const isGradio = config.provider === 'gradio'

  function changeProvider(provider: string) {
    if (provider === 'gradio') {
      onChange({
        ...config,
        provider,
        tts_url: GRADIO_URL_DEFAULT,
        voice: GRADIO_SPEAKER_DEFAULT,
        instructions: GRADIO_INSTRUCTIONS_DEFAULT,
      })
      return
    }
    if (provider === 'vllm') {
      onChange({
        ...config,
        provider,
        tts_url: config.tts_url === GRADIO_URL_DEFAULT ? `http://${window.location.hostname}:8091/v1/audio/speech` : config.tts_url,
        voice: VOICE_SUGGESTIONS.includes(config.voice) ? config.voice : 'ono_anna',
      })
      return
    }
    onChange({ ...config, provider })
  }

  return (
    <div style={{
      display: 'flex', flexDirection: 'column', gap: '14px',
      padding: '16px',
      background: '#fafaf7',
      borderRadius: '10px',
      border: '1px solid #e8e4e0',
    }}>
      {/* Provider selector */}
      <SelectField
        label="🎛 TTS Provider"
        value={config.provider}
        onChange={changeProvider}
        options={[
          { value: 'vllm', label: 'vLLM (Qwen3-TTS)' },
          { value: 'sbv', label: 'style-bert-vits2' },
          { value: 'gradio', label: 'Gradio (/run_instruct)' },
        ]}
        placeholder="-- No TTS --"
      />

      {/* vLLM fields */}
      {isVLLM && (
        <>
          <TextField
            label="🔗 TTS Endpoint URL"
            value={config.tts_url}
            onChange={(v) => onChange({ ...config, tts_url: v })}
            placeholder={`http://${window.location.hostname}:8091/v1/audio/speech`}
          />
          <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap' }}>
            <TextField
              label="🧠 Model"
              value={config.tts_model}
              onChange={(v) => onChange({ ...config, tts_model: v })}
              datalist={MODEL_SUGGESTIONS}
            />
            <SelectField
              label="🎤 Voice"
              value={config.voice}
              onChange={(v) => onChange({ ...config, voice: v })}
              options={VOICE_SUGGESTIONS.map(v => ({ value: v, label: v }))}
            />
          </div>
          <TextField
            label="📝 Instructions"
            value={config.instructions}
            onChange={(v) => onChange({ ...config, instructions: v })}
            placeholder="語速、語調などの指示..."
          />
        </>
      )}

      {/* SBV fields */}
      {isSBV && (
        <>
          <TextField
            label="🔗 Style-Bert-VITS2 API URL"
            value={config.sbv_url}
            onChange={(v) => onChange({ ...config, sbv_url: v })}
            placeholder="http://127.0.0.1:7862"
          />
          <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap' }}>
            <TextField
              label="🧠 Model"
              value={config.sbv_model}
              onChange={(v) => onChange({ ...config, sbv_model: v })}
              datalist={SBV_MODEL_SUGGESTIONS}
            />
            <TextField
              label="🎤 Speaker"
              value={config.sbv_speaker}
              onChange={(v) => onChange({ ...config, sbv_speaker: v })}
              datalist={SBV_SPEAKER_SUGGESTIONS}
            />
            <TextField
              label="🎭 Style"
              value={config.sbv_style}
              onChange={(v) => onChange({ ...config, sbv_style: v })}
              datalist={SBV_STYLE_SUGGESTIONS}
            />
          </div>
        </>
      )}

      {/* Gradio fields */}
      {isGradio && (
        <>
          <TextField
            label="🔗 Gradio API URL"
            value={config.tts_url}
            onChange={(v) => onChange({ ...config, tts_url: v })}
            placeholder={GRADIO_URL_DEFAULT}
          />
          <SelectField
            label="🎤 Speaker"
            value={config.voice}
            onChange={(v) => onChange({ ...config, voice: v })}
            options={GRADIO_SPEAKER_SUGGESTIONS.map(v => ({ value: v, label: v }))}
          />
          <TextField
            label="📝 Instructions"
            value={config.instructions}
            onChange={(v) => onChange({ ...config, instructions: v })}
            placeholder={GRADIO_INSTRUCTIONS_DEFAULT}
          />
        </>
      )}

      {/* Force checkbox */}
      {showForce && config.provider && (
        <label style={{
          display: 'flex', alignItems: 'center', gap: '8px', fontSize: '13px',
          color: '#6b6560', cursor: 'pointer', paddingTop: '4px',
          borderTop: '1px dashed #d9d3cb',
        }}>
          <input
            type="checkbox"
            checked={config.tts_force}
            onChange={(e) => onChange({ ...config, tts_force: e.target.checked })}
            style={{ width: '16px', height: '16px', accentColor: '#4f46e5', cursor: 'pointer' }}
          />
          🔄 Force regenerate (overwrite existing audio files)
        </label>
      )}
    </div>
  )
}
