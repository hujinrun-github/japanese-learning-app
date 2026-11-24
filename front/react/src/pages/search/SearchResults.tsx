import { useId, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/Badge'
import type { GrammarSearchResult, JLPTLevel, Word } from '@/types/api'
import { grammarSearchApiPath } from '@/pages/grammar/grammarSearch'
import { useLookup } from './useLookup'
import styles from './SearchPage.module.css'

export const searchFilters = ['all', 'words', 'grammar'] as const
export type SearchFilter = typeof searchFilters[number]
export type SearchEntry = { type: Exclude<SearchFilter, 'all'>; id: number }
type WordHit = Pick<Word, 'id' | 'kanji_form' | 'reading' | 'meaning' | 'part_of_speech' | 'jlpt_level'>
type SearchCard = { id: number; to: string; title: string; meaning: string; level: JLPTLevel; reading?: string; partOfSpeech?: string }
const previewLimit = 3

interface SearchResultsProps {
  query: string
  filter: SearchFilter
  onFilter: (filter: SearchFilter) => void
  onOpenEntry?: (entry: SearchEntry) => void
}

export function SearchResults({ query, filter, onFilter, onOpenEntry }: SearchResultsProps) {
  const { t } = useTranslation()
  const headingId = useId()
  const wordLookup = useLookup<WordHit[]>(`/api/v1/words/search?q=${encodeURIComponent(query)}`)
  const grammarLookup = useLookup<GrammarSearchResult>(grammarSearchApiPath(query))
  const searchParams = new URLSearchParams({ q: query })
  if (filter !== 'all') searchParams.set('type', filter)

  const wordCards = wordLookup.result.status === 'ready' ? wordLookup.result.data.map((word) => ({
    id: word.id,
    to: `/search/words/${word.id}?${searchParams}`,
    title: word.kanji_form || word.reading,
    reading: word.reading !== word.kanji_form && word.kanji_form ? word.reading : undefined,
    meaning: word.meaning,
    partOfSpeech: word.part_of_speech,
    level: word.jlpt_level,
  })) : []
  const grammarCards = grammarLookup.result.status === 'ready' ? grammarLookup.result.data.points.map((point) => ({
    id: point.id,
    to: `/grammar/${point.id}`,
    title: point.name,
    meaning: point.meaning,
    level: point.jlpt_level,
  })) : []

  const groups = [
    { type: 'words' as const, cards: wordCards, status: wordLookup.result.status, retry: wordLookup.retry },
    { type: 'grammar' as const, cards: grammarCards, status: grammarLookup.result.status, retry: grammarLookup.retry },
  ].filter((group) => filter === 'all' || filter === group.type)

  if (groups.every((group) => group.status === 'ready' && !group.cards.length)) {
    return (
      <div className={styles.feedback} role="status">
        <strong>{t('search.empty', { query })}</strong>
        <p>{t('search.emptyHint')}</p>
      </div>
    )
  }

  const visibleGroups = groups.filter((group) => group.status !== 'ready' || group.cards.length > 0)
  return (
    <div className={!onOpenEntry && visibleGroups.length > 1 ? styles.columns : styles.results}>
      {visibleGroups.map(({ type, cards, status, retry }) => (
        <section className={styles.group} key={type} aria-labelledby={`${headingId}-${type}`}>
          <h2 id={`${headingId}-${type}`}>{t(`search.${type}`)}</h2>
          {status === 'loading' && <p className={styles.feedback} role="status">{t('common.loading')}</p>}
          {status === 'error' && (
            <div className={styles.feedback} role="alert">
              <p>{t('search.unavailable', { type: t(`search.${type}`) })}</p>
              <button className={styles.textButton} type="button" onClick={retry}>{t('search.retry')}</button>
            </div>
          )}
          <ul className={styles.cards}>
            {(filter === 'all' ? cards.slice(0, previewLimit) : cards).map((card: SearchCard) => (
              <li key={card.to}>
                <ResultAction card={card} onClick={onOpenEntry ? () => onOpenEntry({ id: card.id, type }) : undefined}
                  fromSearch={type === 'grammar' ? `/search?${searchParams}` : undefined}>
                  <span className={styles.cardBody}>
                    <span className={styles.cardTitle} lang="ja">
                      <strong>{card.title}</strong>
                      {card.reading && <span>{card.reading}</span>}
                    </span>
                    {card.partOfSpeech && <span className={styles.partOfSpeech}>{card.partOfSpeech}</span>}
                    <span className={styles.cardMeaning}>{card.meaning}</span>
                  </span>
                  <Badge level={card.level} size="sm" />
                </ResultAction>
              </li>
            ))}
          </ul>
          {filter === 'all' && cards.length > previewLimit && (
            <button className={styles.textButton} type="button" onClick={() => onFilter(type)}>
              {t('search.more', { type: t(`search.${type}`) })} <span aria-hidden="true">→</span>
            </button>
          )}
          {filter !== 'all' && cards.length >= 50 && <p className={styles.limit}>{t('search.limited')}</p>}
        </section>
      ))}
    </div>
  )
}

function ResultAction({ card, onClick, fromSearch, children }: {
  card: SearchCard; onClick?: () => void; fromSearch?: string; children: ReactNode
}) {
  if (onClick) return <button type="button" className={styles.card} onClick={onClick}>{children}</button>
  return <Link className={styles.card} to={card.to} state={fromSearch ? { fromSearch } : undefined}>{children}</Link>
}
