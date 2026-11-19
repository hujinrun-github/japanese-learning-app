export type ErrataContentField = {
  key: string
  label: string
  value: string
  kind?: 'text' | 'textarea' | 'json'
  readonly?: boolean
}

export type ErrataAudioTarget = {
  key: string
  label: string
  text: string
  audio_url: string
}

export type ErrataContent = {
  module: string
  item_key: string
  title: string
  revision: string
  fields: ErrataContentField[]
  audio_targets: ErrataAudioTarget[]
}

export function contentDraft(fields: ErrataContentField[]): Record<string, string> {
  return Object.fromEntries(fields.map(field => [field.key, field.value]))
}

export function changedContentValues(fields: ErrataContentField[], draft: Record<string, string>): Record<string, string> {
  return Object.fromEntries(fields
    .filter(field => !field.readonly && draft[field.key] !== undefined && draft[field.key] !== field.value)
    .map(field => [field.key, draft[field.key]]))
}

export function invalidJSONField(fields: ErrataContentField[], draft: Record<string, string>): string | null {
  for (const field of fields) {
    if (field.kind !== 'json' || field.readonly) continue
    try {
      JSON.parse(draft[field.key] ?? field.value)
    } catch {
      return field.label
    }
  }
  return null
}

export function cacheBustAudioURL(url: string, version: number): string {
  const fragmentIndex = url.indexOf('#')
  const path = fragmentIndex === -1 ? url : url.slice(0, fragmentIndex)
  const fragment = fragmentIndex === -1 ? '' : url.slice(fragmentIndex)
  return `${path}${path.includes('?') ? '&' : '?'}t=${version}${fragment}`
}

type ResolutionState = {
  loaded: boolean
  busy: boolean
  dirty: boolean
  audioDirty: boolean
  error: string
  audioRequired: boolean
  audioVerified: boolean
  confirmed: boolean
}

export function resolutionBlockedReason(state: ResolutionState): string | null {
  if (!state.loaded) return '请先加载原内容。'
  if (state.busy) return '正在保存，请稍候。'
  if (state.dirty) return '请先保存原内容的修改。'
  if (state.audioDirty) return '朗读文本已修改，请先重新生成并试听。'
  if (state.error) return '请先解决保存或试听时遇到的问题。'
  if (state.audioRequired && !state.audioVerified) return '请先完整试听待确认的音频。'
  if (!state.confirmed) return '请确认原内容中的问题已修复。'
  return null
}
