import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/Badge'
import { DictionaryEntryButton } from '@/pages/search/ContextualSearch'
import type { PracticeKnowledgeLink } from '@/types/api'
import { useLookup } from './useLookup'
import styles from './PracticeKnowledgeLinks.module.css'

export function PracticeKnowledgeLinks({ links, hints }: { links?: PracticeKnowledgeLink[]; hints?: boolean }) {
  const { t } = useTranslation()
  const heading = useId()
  if (!links?.length) return null

  return (
    <section className={styles.panel} aria-labelledby={heading}>
      <h3 className={styles.title} id={heading}>{t(hints ? 'search.knowledgeHints' : 'search.knowledgeLinks')}</h3>
      <ul className={styles.list}>
        {links.map(({ kind, id, title, reading, meaning, jlpt_level }) => (
          <li key={`${kind}:${id}`}>
            <DictionaryEntryButton
              entry={{ type: kind === 'word' ? 'words' : 'grammar', id }}
              className={styles.entry}
            >
              <span className={styles.kind}>{t(kind === 'word' ? 'search.words' : 'search.grammar')}</span>
              <strong lang="ja">{title}</strong>
              {reading && reading !== title && <span className={styles.description} lang="ja">{reading}</span>}
              <Badge level={jlpt_level} size="sm" />
              <span className={styles.description}>{meaning}</span>
              <span className={styles.arrow} aria-hidden="true">›</span>
            </DictionaryEntryButton>
          </li>
        ))}
      </ul>
    </section>
  )
}

export function RemotePracticeKnowledgeLinks({ path, hints }: { path: string; hints?: boolean }) {
  const { result } = useLookup<PracticeKnowledgeLink[]>(path)
  return result.status === 'ready' ? <PracticeKnowledgeLinks links={result.data} hints={hints} /> : null
}
