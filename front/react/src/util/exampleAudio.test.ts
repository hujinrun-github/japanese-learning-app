import { describe, expect, it } from 'vitest'
import { getWordAudioURL } from './exampleAudio'

describe('getWordAudioURL', () => {
  const tests = [
    { name: 'prefixes a stored filename', input: 'word.wav', want: '/audio/words/word.wav' },
    { name: 'keeps an API path', input: '/api/v1/audio/12', want: '/api/v1/audio/12' },
    { name: 'keeps an audio path', input: '/audio/words/word.wav', want: '/audio/words/word.wav' },
    { name: 'keeps an absolute URL', input: 'https://cdn.example.com/word.wav', want: 'https://cdn.example.com/word.wav' },
  ]

  for (const tt of tests) {
    it(tt.name, () => {
      expect(getWordAudioURL(tt.input)).toBe(tt.want)
    })
  }
})
