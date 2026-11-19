import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { SpeakingText } from './SpeakingText'
import { PronunciationFeedback } from './PronunciationFeedback'
import type { PronunciationResult } from '@/types/speaking'

void i18next.use(initReactI18next).init({ lng: 'zh', resources: {} })

const pronunciation: PronunciationResult = {
  status: 'available', words: [
    { start: 3, end: 4, surface: '駅', reading: 'えき', recognized: 'いき', status: 'mismatch' },
    { start: 4, end: 5, surface: 'へ', reading: 'え', recognized: '', status: 'missing' },
    { start: 5, end: 7, surface: '行く', reading: 'いく', recognized: 'いく', status: 'correct' },
  ],
}

describe('speaking pronunciation marks', () => {
  it('marks whole erroneous words using original codepoint offsets and preserves furigana', () => {
    const html = renderToStaticMarkup(<SpeakingText
      lines={['駅へ行く。']} annotations={[]} offsets={[3]} pronunciation={pronunciation}
      activeSentenceIndex={0} onPlaySentence={() => {}}
    />)
    expect(html).toContain('aria-current="true"')
    expect(html).toContain('data-pronunciation="mismatch"')
    expect(html).toContain('data-pronunciation="missing"')
    expect(html.match(/<mark\b/g)).toHaveLength(2)
    expect(html).toContain('駅<rt>えき</rt>')
    expect(html).toContain('行く<rt>いく</rt>')
    expect(html).toContain('いき')
    expect(html).toContain('播放第 1 句')
  })

  it.each(['unavailable', 'error'] as const)('does not mark any word when recognition is %s', (status) => {
    const result = { ...pronunciation, status }
    const html = renderToStaticMarkup(<SpeakingText lines={['駅へ行く。']} annotations={[]} offsets={[3]} pronunciation={result} />)
    expect(html).not.toContain('<mark')
    expect(html).toContain('駅へ行く。')
    const feedback = renderToStaticMarkup(<PronunciationFeedback result={result} recognizedText="" />)
    expect(feedback).not.toContain('未发现')
  })

  it('preserves whitespace and supplementary Unicode before erroneous words', () => {
    const result: PronunciationResult = { status: 'available', words: [
      { start: 2, end: 3, surface: '駅', reading: 'えき', status: 'missing' },
    ] }
    const html = renderToStaticMarkup(<SpeakingText lines={['😀 駅。']} annotations={[]} offsets={[0]} pronunciation={result} />)
    expect(html).toContain('😀 ')
    expect(html).toContain('駅<rt>えき</rt>')
    expect(html).toContain('data-pronunciation="missing"')
  })

  it('ignores offsets that do not belong to the current sentence', () => {
    const html = renderToStaticMarkup(<SpeakingText lines={['猫です。']} annotations={[]} offsets={[3]} pronunciation={pronunciation} />)
    expect(html).not.toContain('<mark')
    expect(html).toContain('猫です。')
  })

  it('distinguishes insufficient audio from a disabled recognition service', () => {
    const html = renderToStaticMarkup(<PronunciationFeedback
      result={{ status: 'unavailable', reason: 'insufficient_audio', words: [] }} recognizedText="" />)
    expect(html).toContain('录音不足以判断')
    expect(html).not.toContain('暂未启用')
  })

  it('reports possible mistakes and recognized transcript without implying phoneme precision', () => {
    const html = renderToStaticMarkup(<PronunciationFeedback result={pronunciation} recognizedText="いき行く" />)
    expect(html).toContain('2')
    expect(html).toContain('疑似读错')
    expect(html).toContain('语音识别')
    expect(html).toContain('いき行く')
  })
})
