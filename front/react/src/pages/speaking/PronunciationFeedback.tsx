import { useTranslation } from 'react-i18next'
import type { PronunciationResult } from '@/types/speaking'
import styles from './SpeakingPage.module.css'

export function PronunciationFeedback({ result, recognizedText }: {
  result?: PronunciationResult
  recognizedText: string
}) {
  const { t } = useTranslation()
  if (!result) return null
  const mistakes = result.words.filter((word) => word.status !== 'correct')
  return (
    <section className={styles.pronunciationFeedback} aria-label={t('speaking.pronunciation.title', { defaultValue: '单词读音检查' })}>
      <h2>{t('speaking.pronunciation.title', { defaultValue: '单词读音检查' })}</h2>
      {result.status === 'available' ? (
        <>
          <p>{mistakes.length
            ? t('speaking.pronunciation.mistakes', { count: mistakes.length, defaultValue: '发现 {{count}} 处疑似读错或漏读，请查看原文标记。' })
            : t('speaking.pronunciation.noMistakes', { defaultValue: '未发现明显的读音差异。' })}</p>
          <p className={styles.pronunciationHint}>{t('speaking.pronunciation.hint', { defaultValue: '红色下划线：疑似读错；虚线：疑似漏读。结果基于语音识别，请结合原声复听。' })}</p>
          {recognizedText && <details>
            <summary>{t('speaking.pronunciation.transcript', { defaultValue: '查看识别文本' })}</summary>
            <p lang="ja">{recognizedText}</p>
          </details>}
        </>
      ) : <p role="status">{t(`speaking.pronunciation.${result.reason === 'insufficient_audio' ? 'insufficientAudio' : result.status}`, {
        defaultValue: result.reason === 'insufficient_audio'
          ? '本次录音不足以判断单词读音，请查看录音提示后重试。'
          : result.status === 'unavailable'
          ? '单词读音识别暂未启用，本次仅提供节奏、语调等评分。'
          : '单词读音识别暂时失败，请重新录音；本次仍可查看节奏、语调等评分。',
      })}</p>}
    </section>
  )
}
