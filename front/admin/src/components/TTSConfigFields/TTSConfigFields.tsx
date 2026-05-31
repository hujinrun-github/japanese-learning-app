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

export const defaultTTSConfig: TTSConfig = {
  provider: '',
  tts_url: 'http://192.168.1.16:8091/v1/audio/speech',
  tts_model: 'Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice',
  voice: 'ono_anna',
  instructions: '標準語で、ゆっくり、はっきり発音してください。単語のあとに少し間を空けてください。',
  sbv_url: 'http://127.0.0.1:7862',
  sbv_model: 'amitaro',
  sbv_speaker: 'あみたろ',
  sbv_style: 'Neutral',
  tts_force: false,
}

interface Props {
  config: TTSConfig
  onChange: (c: TTSConfig) => void
  showForce?: boolean
}

export function TTSConfigFields({ config, onChange, showForce }: Props) {
  const isVLLM = config.provider === 'vllm'
  const isSBV = config.provider === 'sbv'

  return (
    <div style={{
      display: 'flex',
      flexWrap: 'wrap',
      gap: '8px',
      alignItems: 'center',
      padding: '8px 0',
      borderTop: '1px solid #e2e8f0',
      marginTop: '4px',
    }}>
      <select
        value={config.provider}
        onChange={(e) => onChange({ ...config, provider: e.target.value })}
        style={{ padding: '6px 8px', fontSize: '12px', border: '1px solid #cbd5e1', borderRadius: '4px' }}
      >
        <option value="">-- No TTS --</option>
        <option value="vllm">vLLM (Qwen3-TTS)</option>
        <option value="sbv">style-bert-vits2</option>
      </select>

      {isVLLM && (
        <>
          <input
            placeholder="TTS URL"
            value={config.tts_url}
            onChange={(e) => onChange({ ...config, tts_url: e.target.value })}
            style={{ padding: '6px 8px', fontSize: '12px', border: '1px solid #cbd5e1', borderRadius: '4px', minWidth: '200px' }}
          />
          <input
            placeholder="Model"
            value={config.tts_model}
            onChange={(e) => onChange({ ...config, tts_model: e.target.value })}
            style={{ padding: '6px 8px', fontSize: '12px', border: '1px solid #cbd5e1', borderRadius: '4px', minWidth: '180px' }}
          />
          <input
            placeholder="Voice"
            value={config.voice}
            onChange={(e) => onChange({ ...config, voice: e.target.value })}
            style={{ padding: '6px 8px', fontSize: '12px', border: '1px solid #cbd5e1', borderRadius: '4px', width: '100px' }}
          />
          <input
            placeholder="Instructions"
            value={config.instructions}
            onChange={(e) => onChange({ ...config, instructions: e.target.value })}
            style={{ padding: '6px 8px', fontSize: '12px', border: '1px solid #cbd5e1', borderRadius: '4px', minWidth: '240px' }}
          />
        </>
      )}

      {isSBV && (
        <>
          <input
            placeholder="SBV URL"
            value={config.sbv_url}
            onChange={(e) => onChange({ ...config, sbv_url: e.target.value })}
            style={{ padding: '6px 8px', fontSize: '12px', border: '1px solid #cbd5e1', borderRadius: '4px', minWidth: '180px' }}
          />
          <input
            placeholder="Model"
            value={config.sbv_model}
            onChange={(e) => onChange({ ...config, sbv_model: e.target.value })}
            style={{ padding: '6px 8px', fontSize: '12px', border: '1px solid #cbd5e1', borderRadius: '4px', width: '120px' }}
          />
          <input
            placeholder="Speaker"
            value={config.sbv_speaker}
            onChange={(e) => onChange({ ...config, sbv_speaker: e.target.value })}
            style={{ padding: '6px 8px', fontSize: '12px', border: '1px solid #cbd5e1', borderRadius: '4px', width: '120px' }}
          />
          <input
            placeholder="Style"
            value={config.sbv_style}
            onChange={(e) => onChange({ ...config, sbv_style: e.target.value })}
            style={{ padding: '6px 8px', fontSize: '12px', border: '1px solid #cbd5e1', borderRadius: '4px', width: '100px' }}
          />
        </>
      )}

      {showForce && config.provider && (
        <label style={{ display: 'flex', alignItems: 'center', gap: '4px', fontSize: '12px', color: '#64748b', cursor: 'pointer' }}>
          <input
            type="checkbox"
            checked={config.tts_force}
            onChange={(e) => onChange({ ...config, tts_force: e.target.checked })}
          />
          Force regenerate
        </label>
      )}
    </div>
  )
}
