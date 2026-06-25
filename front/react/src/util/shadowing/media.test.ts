import { describe, expect, test } from 'vitest'

import { selectShadowingMedia } from './media'

describe('selectShadowingMedia', () => {
  test('uses video when a video URL is available', () => {
    expect(selectShadowingMedia({
      audio_url: '/audio/lesson.wav',
      video_url: '/api/v1/videos/1/stream',
      shadowing_config: { media_type: 'video' },
    })).toEqual({
      kind: 'video',
      url: '/api/v1/videos/1/stream',
    })
  })

  test('falls back to audio when video URL is absent', () => {
    expect(selectShadowingMedia({
      audio_url: '/audio/lesson.wav',
      video_url: '',
      shadowing_config: { media_type: 'audio' },
    })).toEqual({
      kind: 'audio',
      url: '/audio/lesson.wav',
    })
  })
})
