import { createContext, lazy, Suspense, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { getDictionarySelection, selectionKeyword } from './contextQuery'
import type { SearchEntry } from './SearchResults'
import styles from './ContextualSearch.module.css'

export type ContextualSearchTarget = { source?: string; keyword?: string; entry?: SearchEntry }
const OpenSearchContext = createContext<((target: ContextualSearchTarget) => void) | null>(null)
const ContextualSearchDialog = lazy(() => import('./ContextualSearchDialog'))

export function ContextualSearchProvider({ children }: { children: ReactNode }) {
  const [target, setTarget] = useState<ContextualSearchTarget | null>(null)
  const openSearch = useCallback((next: ContextualSearchTarget) => {
    setTarget((current) => next.source || next.keyword || next.entry ? next : current ?? next)
  }, [])

  return (
    <OpenSearchContext.Provider value={openSearch}>
      {children}
      {!target && <SelectionAction />}
      {target && (
        <Suspense fallback={null}>
          <ContextualSearchDialog target={target} onClose={() => setTarget(null)} />
        </Suspense>
      )}
    </OpenSearchContext.Provider>
  )
}

export function FloatingSearchButton({ className, children, onOpen }: {
  className: string; children: ReactNode; onOpen: () => void
}) {
  const openSearch = useContext(OpenSearchContext)
  return (
    <button type="button" className={className} aria-haspopup="dialog"
      onClick={(event) => {
        event.currentTarget.focus({ preventScroll: true })
        onOpen()
        openSearch?.({})
      }}>
      {children}
    </button>
  )
}

export function DictionaryEntryButton({ entry, className, children }: {
  entry: SearchEntry; className: string; children: ReactNode
}) {
  const openSearch = useContext(OpenSearchContext)
  return (
    <button type="button" className={className} aria-haspopup="dialog" onClick={(event) => {
      event.currentTarget.focus({ preventScroll: true })
      openSearch?.({ entry })
    }}>{children}</button>
  )
}

export function DictionaryButton({ text, selected, compact, onOpen }: {
  text?: string; selected?: boolean; compact?: boolean; onOpen?: () => void
}) {
  const { t } = useTranslation()
  const openSearch = useContext(OpenSearchContext)
  const label = t('search.inContext')
  return (
    <button type="button" className={styles.trigger} aria-haspopup="dialog"
      aria-label={label} title={compact ? label : undefined}
      onPointerDown={selected ? (event) => event.preventDefault() : undefined}
      onClick={(event) => {
        event.stopPropagation()
        const keyword = selected ? selectionKeyword(text ?? '') : undefined
        window.getSelection()?.removeAllRanges()
        openSearch?.({ keyword, source: keyword ? undefined : text })
        onOpen?.()
      }}>
      <span aria-hidden="true">🔍</span>{!compact && ` ${label}`}
    </button>
  )
}

function SelectionAction() {
  const { t } = useTranslation()
  const [keyword, setKeyword] = useState('')

  useEffect(() => {
    function update() { setKeyword(getDictionarySelection(window.getSelection())) }
    document.addEventListener('selectionchange', update)
    return () => document.removeEventListener('selectionchange', update)
  }, [])

  if (!keyword) return null
  return (
    <div className={styles.selection}>
      <span className={styles.selectedText} lang="ja">{keyword}</span>
      <DictionaryButton text={keyword} selected />
      <button type="button" className={styles.close}
        aria-label={t('search.dismissSelection')}
        onClick={() => {
          window.getSelection()?.removeAllRanges()
          setKeyword('')
        }}>×</button>
    </div>
  )
}
