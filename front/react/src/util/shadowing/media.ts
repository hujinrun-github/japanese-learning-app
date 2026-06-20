type ShadowingMediaLesson = {
  audio_url?: string
  video_url?: string
  shadowing_config?: Record<string, unknown>
}

export type ShadowingMedia = {
  kind: 'audio' | 'video'
  url: string
}

export function selectShadowingMedia(lesson: ShadowingMediaLesson): ShadowingMedia {
  const mediaType = typeof lesson.shadowing_config?.media_type === 'string'
    ? lesson.shadowing_config.media_type
    : ''
  const videoURL = lesson.video_url?.trim() ?? ''
  const audioURL = lesson.audio_url?.trim() ?? ''

  if (videoURL && mediaType === 'video') {
    return { kind: 'video', url: videoURL }
  }
  if (audioURL) {
    return { kind: 'audio', url: audioURL }
  }
  if (videoURL) {
    return { kind: 'video', url: videoURL }
  }
  return { kind: 'audio', url: '' }
}
