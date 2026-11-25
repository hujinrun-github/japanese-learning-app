import { Link, useLocation, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/Badge'
import { ErrataButton } from '@/components/ui/ErrataButton'
import { useSourceAnchor } from '@/hooks/useSourceAnchor'
import type { Word } from '@/types/api'
import { speakExample, speakWord } from '@/util/exampleAudio'
import { DictionaryButton } from './ContextualSearch'
import { useLookup } from './useLookup'
import styles from './SearchPage.module.css'

export default function WordEntryPage() {
  const { t } = useTranslation()
  const { id } = useParams()
  const { search, hash } = useLocation()
  const { result, retry } = useLookup<Word>(`/api/v1/words/${encodeURIComponent(id ?? '')}`)
  const anchor = /^#(?:examples|example-[1-9]\d*)$/.test(hash) ? hash.slice(1) : null
  useSourceAnchor(anchor, result.status === 'ready' && result.data.id === Number(id), 'examples')

  return (
    <div className={styles.page}>
      <Link className={styles.back} to={`/search${search}`}>← {t('search.back')}</Link>
      {result.status === 'loading' && <p className={styles.feedback} role="status">{t('common.loading')}</p>}
      {result.status === 'error' && (
        <div className={styles.feedback} role="alert">
          <p>{t('search.entryUnavailable')}</p>
          <button className={styles.textButton} onClick={retry}>{t('search.retry')}</button>
        </div>
      )}
      {result.status === 'ready' && <WordEntry word={result.data} />}
    </div>
  )
}

export function WordEntry({ word, contextualLookup = true }: { word: Word; contextualLookup?: boolean }) {
  const { t } = useTranslation()
  const title = word.kanji_form || word.reading
  const traits = [word.part_of_speech, word.verb_type].filter(Boolean)
  return (
    <article className={styles.entry}>
      <header className={styles.entryHead}>
        <div>
          <h1 lang="ja">{title}</h1>
          {word.reading !== title && <p lang="ja">{word.reading}</p>}
        </div>
        <Badge level={word.jlpt_level} />
      </header>
      <button type="button" className={styles.audio} onClick={() => void speakWord(word.audio_url, word.reading || title)}>
        <span aria-hidden="true">🔊</span> {t('search.pronunciation')}
      </button>
      {traits.length > 0 && <p className={styles.partOfSpeech}>{traits.join(' · ')}</p>}
      <p className={styles.definition}>{word.meaning}</p>
      {word.examples?.length > 0 && (
        <section className={styles.examples} id={contextualLookup ? 'examples' : undefined} tabIndex={contextualLookup ? -1 : undefined}>
          <h2>{t('search.examples')}</h2>
          <ol>
            {word.examples.map((example, index) => (
              <li key={example.id ?? index}
                id={contextualLookup && example.id ? `example-${example.id}` : undefined}
                tabIndex={contextualLookup && example.id ? -1 : undefined}>
                <div>
                  {example.furigana_html ? (
                    <p lang="ja" data-dictionary-text={contextualLookup ? '' : undefined}
                      dangerouslySetInnerHTML={{ __html: example.furigana_html }} />
                  ) : (
                    <p lang="ja" data-dictionary-text={contextualLookup ? '' : undefined}>{example.japanese}</p>
                  )}
                  <p>{example.chinese}</p>
                  {contextualLookup && <DictionaryButton text={example.japanese} />}
                  {contextualLookup && example.id && (
                    <ErrataButton module="word" itemKey={`${word.id}:example:${example.id}`} label={example.japanese} />
                  )}
                </div>
                <button type="button" className={styles.audio} aria-label={`${t('search.playExample')} ${index + 1}`}
                  onClick={() => void speakExample(example.japanese)}>🔊</button>
              </li>
            ))}
          </ol>
        </section>
      )}
    </article>
  )
}
