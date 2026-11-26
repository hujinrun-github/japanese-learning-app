import test from 'node:test'
import assert from 'node:assert/strict'
import { wordAudioPlaybackURL, wordAudioUrl } from './audioHash.ts'

for (const [name, input, expected] of [
  ['normalized path from words API', '/audio/words/a98218d942dea213.wav', '/audio/words/a98218d942dea213.wav'],
  ['object-store stream path', '/api/v1/audio/332/stream', '/api/v1/audio/332/stream'],
  ['absolute URL', 'https://cdn.example.com/a.wav', 'https://cdn.example.com/a.wav'],
  ['legacy bare filename', 'local.wav', '/audio/words/local.wav'],
  ['padded value', '  /audio/words/x.wav  ', '/audio/words/x.wav'],
  ['empty string falls back to hash', '', undefined],
  ['undefined falls back to hash', undefined, undefined],
  ['null falls back to hash', null, undefined],
]) {
  test(`wordAudioPlaybackURL: ${name}`, () => {
    assert.equal(wordAudioPlaybackURL(input), expected)
  })
}

test('regression: an API path must not be prefixed a second time', () => {
  const fromApi = '/audio/words/b7e853b7a4f7605b.wav'
  assert.equal(wordAudioPlaybackURL(fromApi), fromApi)
  assert.equal(wordAudioUrl(fromApi), '/audio/words//audio/words/b7e853b7a4f7605b.wav')
})
