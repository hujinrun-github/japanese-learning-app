import test from 'node:test'
import assert from 'node:assert/strict'
import {
  cacheBustAudioURL,
  changedContentValues,
  invalidJSONField,
  resolutionBlockedReason,
} from './errataCorrectionRules.ts'

const fields = [
  { key: 'word:3.meaning', label: '中文释义', value: '工厂', kind: 'text' },
  { key: 'word_example:9.japanese', label: '例句 1', value: '町に工場がある。', kind: 'textarea' },
  { key: 'translation:1.source_text', label: '原文', value: '原文', readonly: true },
]

test('save sends only changed editable source fields', () => {
  assert.deepEqual(changedContentValues(fields, {
    'word:3.meaning': '工厂',
    'word_example:9.japanese': '町に工場があります。',
    'translation:1.source_text': '不能覆盖练习原文',
    unknown: 'cannot inject a field',
  }), { 'word_example:9.japanese': '町に工場があります。' })
})

test('clearing a field is a change, missing drafts are not', () => {
  assert.deepEqual(changedContentValues(fields, { 'word:3.meaning': '' }), { 'word:3.meaning': '' })
})

test('invalid JSON is caught before saving actual content', () => {
  const options = [{ key: 'quiz:1.options', label: '选项', value: '["A"]', kind: 'json' }]
  assert.equal(invalidJSONField(options, { 'quiz:1.options': '[invalid' }), '选项')
  assert.equal(invalidJSONField(options, { 'quiz:1.options': '["B"]' }), null)
})

for (const [url, expected] of [
  ['/audio/examples/file.wav', '/audio/examples/file.wav?t=123'],
  ['/audio/examples/file.wav?token=abc', '/audio/examples/file.wav?token=abc&t=123'],
  ['/audio/examples/file.wav?token=abc#clip', '/audio/examples/file.wav?token=abc&t=123#clip'],
]) {
  test(`cache busting preserves the playback URL: ${url}`, () => {
    assert.equal(cacheBustAudioURL(url, 123), expected)
  })
}

const ready = {
  loaded: true,
  busy: false,
  dirty: false,
  audioDirty: false,
  error: '',
  audioRequired: false,
  audioVerified: false,
  confirmed: true,
}

for (const [name, state] of [
  ['unloaded content', { loaded: false }],
  ['in-flight save', { busy: true }],
  ['unsaved source edits', { dirty: true }],
  ['unsynthesized pronunciation edits', { audioDirty: true }],
  ['failed save or playback', { error: 'failed' }],
  ['missing explicit confirmation', { confirmed: false }],
  ['audio report without a completed listen', { audioRequired: true }],
]) {
  test(`cannot mark resolved with ${name}`, () => {
    assert.equal(typeof resolutionBlockedReason({ ...ready, ...state }), 'string')
  })
}

test('text correction can resolve after checking the saved original content', () => {
  assert.equal(resolutionBlockedReason(ready), null)
})

test('audio correction can resolve only after listening and explicit confirmation', () => {
  assert.equal(resolutionBlockedReason({ ...ready, audioRequired: true, audioVerified: true }), null)
})
