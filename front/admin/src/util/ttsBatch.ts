import type { TTSConfig } from '@/components/TTSConfigFields/TTSConfigFields'

export interface BatchAudioResponse {
  module: string
  total: number
  generated: number
  existing: number
  failed: number
}

interface BatchAudioFilters {
  level?: string
  type?: string
  texts?: string[]
  wordIds?: number[]
}

export function buildBatchAudioPayload(module: 'words' | 'grammar' | 'speaking', config: TTSConfig, filters: BatchAudioFilters = {}) {
  return {
    module,
    level: filters.level || '',
    type: filters.type || '',
    texts: filters.texts || [],
    word_ids: filters.wordIds || [],
    force: config.tts_force,
    provider: config.provider,
    tts_url: config.tts_url,
    tts_model: config.tts_model,
    voice: config.voice,
    instructions: config.instructions,
    sbv_url: config.sbv_url,
    sbv_model: config.sbv_model,
    sbv_speaker: config.sbv_speaker,
    sbv_style: config.sbv_style,
  }
}

export function formatBatchAudioResult(label: string, result: BatchAudioResponse) {
  return `${label} audio complete: ${result.generated} generated, ${result.existing} already existed, ${result.failed} failed, ${result.total} total.`
}
