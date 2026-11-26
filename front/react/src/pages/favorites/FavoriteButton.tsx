import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useFavorites } from './FavoritesProvider'
import { favoriteKey, type FavoriteTarget } from './favorites'
import styles from './FavoritesPage.module.css'

export function FavoriteButton({ target, showRemove = false }: { target: FavoriteTarget; showRemove?: boolean }) {
  const { t } = useTranslation()
  const favorites = useFavorites()
  const id = favoriteKey(target)
  const [failedTarget, setFailedTarget] = useState('')
  if (!favorites) return null
  const saved = favorites.keys.has(id)
  const busy = favorites.pending.has(id)
  const failed = failedTarget === id
  const sentence = target.kind === 'listening_sentence'
  const label = favorites.status === 'error' ? t('favorites.retry')
    : saved && showRemove ? t('favorites.remove')
      : saved ? t(sentence ? 'favorites.savedSentence' : 'favorites.saved')
      : t(sentence ? 'favorites.saveSentence' : 'favorites.save')

  async function update() {
    if (!favorites) return
    if (favorites.status === 'error') {
      favorites.retry()
      return
    }
    setFailedTarget('')
    try {
      await favorites.setSaved(target, !saved)
    } catch (error) {
      console.error('Could not update favorite', error)
      setFailedTarget(id)
    }
  }

  return <span className={styles.control}>
    <button className={styles.favoriteButton} type="button" aria-pressed={saved}
      disabled={busy || favorites.status === 'loading'} onClick={event => { event.stopPropagation(); void update() }}
      title={saved ? t('favorites.remove') : label}>
      <span aria-hidden="true">{saved ? '⭐' : '☆'}</span> {label}
    </button>
    {failed && <span role="alert" className={styles.error}>{t('favorites.updateError')}</span>}
  </span>
}
