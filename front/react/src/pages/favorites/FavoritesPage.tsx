import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/Badge'
import type { JLPTLevel } from '@/types/api'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { FavoriteButton } from './FavoriteButton'
import { useFavorites } from './FavoritesProvider'
import { favoriteKey, favoriteKinds, type FavoriteKind } from './favorites'
import styles from './FavoritesPage.module.css'

const filters = ['all', ...favoriteKinds.filter(kind => kind !== 'listening_sentence')] as const

export function FavoritesPage() {
  const { t } = useTranslation()
  const favorites = useFavorites()
  const [kind, setKind] = useState<FavoriteKind | 'all'>('all')
  const [search, setSearch] = useState('')
  const query = search.trim().toLocaleLowerCase()
  const results = (favorites?.items ?? []).filter(item => {
    const inModule = kind === 'all' || kind === item.kind || kind === 'listening' && item.kind === 'listening_sentence'
    return inModule && (!query || `${item.title} ${item.detail}`.toLocaleLowerCase().includes(query))
  }).sort((a, b) => a.title.localeCompare(b.title))
  const ready = favorites?.status === 'ready'

  return <div className={styles.page}>
    <header className={styles.header}>
      <h1>{t('nav.favorites')}</h1>
      <input type="search" value={search} aria-label={t('favorites.search')}
        placeholder={t('favorites.search')} onChange={event => setSearch(event.target.value)} />
    </header>
    <div className={styles.filters} role="group" aria-label={t('favorites.filter')}>
      {filters.map(filter => <button key={filter} type="button" aria-pressed={filter === kind}
        onClick={() => setKind(filter)}>{t(`favorites.kinds.${filter}`)}</button>)}
    </div>
    {!favorites || favorites.status === 'loading' ? <div className={styles.center}><Spinner /></div> : null}
    {favorites?.status === 'error' && <div className={styles.center} role="alert">
      <p>{t('favorites.loadError')}</p>
      <button type="button" className={styles.favoriteButton} onClick={favorites.retry}>{t('favorites.retry')}</button>
    </div>}
    {ready && results.length === 0 && <EmptyState icon="⭐"
      title={t(favorites.items.length ? 'favorites.noMatches' : 'favorites.empty')}
      description={t(favorites.items.length ? 'favorites.tryFilters' : 'favorites.emptyHint')} />}
    {ready && results.length > 0 && <>
      <p className={styles.count} role="status">{t('favorites.count', { count: results.length })}</p>
      <ul className={styles.results}>
        {results.map(item => <li className={styles.card} key={favoriteKey(item)}>
          <div className={styles.meta}><span>{t(`favorites.kinds.${item.kind}`)}</span>
            {item.jlpt_level && <Badge level={item.jlpt_level as JLPTLevel} size="sm" />}</div>
          <h2>{item.path ? item.title : t('favorites.unavailable')}</h2>
          {item.detail && <p className={styles.detail}>{item.detail}</p>}
          <div className={styles.actions}>
            {item.path && <Link to={item.path} aria-label={`${t('favorites.open')} · ${item.title}`}>{t('favorites.open')} →</Link>}
            <FavoriteButton target={item} showRemove />
          </div>
        </li>)}
      </ul>
    </>}
  </div>
}
