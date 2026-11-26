import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { apiFetch } from '@/api/client'
import { favoriteKey, type FavoriteItem, type FavoriteTarget } from './favorites'

interface FavoritesState {
  items: FavoriteItem[]
  keys: ReadonlySet<string>
  pending: ReadonlySet<string>
  status: 'loading' | 'ready' | 'error'
  retry: () => void
  setSaved: (target: FavoriteTarget, saved: boolean) => Promise<void>
}

const FavoritesContext = createContext<FavoritesState | null>(null)

export function useFavorites() {
  return useContext(FavoritesContext)
}

export function FavoritesProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<FavoriteItem[]>([])
  const [status, setStatus] = useState<FavoritesState['status']>('loading')
  const [attempt, setAttempt] = useState(0)
  const [pending, setPending] = useState<ReadonlySet<string>>(() => new Set())
  const inFlight = useRef(new Set<string>())
  const retry = useCallback(() => setAttempt(value => value + 1), [])
  const keys = useMemo(() => new Set(items.map(favoriteKey)), [items])

  useEffect(() => {
    const controller = new AbortController()
    setStatus('loading')
    apiFetch<FavoriteItem[]>('GET', '/api/v1/favorites', undefined, controller.signal)
      .then(data => {
        if (controller.signal.aborted) return
        setItems(data ?? [])
        setStatus('ready')
      })
      .catch(error => {
        if (controller.signal.aborted) return
        console.error('Could not load favorites', error)
        setStatus('error')
      })
    return () => controller.abort()
  }, [attempt])

  const setSaved = useCallback(async (target: FavoriteTarget, saved: boolean) => {
    const id = favoriteKey(target)
    if (inFlight.current.has(id)) return
    inFlight.current.add(id)
    setPending(new Set(inFlight.current))
    try {
      const updated = await apiFetch<FavoriteItem | undefined>(saved ? 'PUT' : 'DELETE',
        `/api/v1/favorites/${target.kind}/${encodeURIComponent(target.key)}`)
      setItems(current => {
        const remaining = current.filter(item => favoriteKey(item) !== id)
        return updated ? [...remaining, updated] : remaining
      })
    } finally {
      inFlight.current.delete(id)
      setPending(new Set(inFlight.current))
    }
  }, [])

  const value = useMemo(() => ({ items, keys, pending, status, retry, setSaved }),
    [items, keys, pending, status, retry, setSaved])
  return <FavoritesContext.Provider value={value}>{children}</FavoritesContext.Provider>
}
