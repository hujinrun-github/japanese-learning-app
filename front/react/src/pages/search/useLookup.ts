import { useEffect, useState } from 'react'
import { apiFetch } from '@/api/client'

type LookupResult<T> = { status: 'loading' } | { status: 'error' } | { status: 'ready'; data: T }

export function useLookup<T>(path: string | null) {
  const [response, setResponse] = useState<{ path: string; result: LookupResult<T> }>()
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    if (!path) return
    const controller = new AbortController()
    apiFetch<T>('GET', path, undefined, controller.signal).then(
      (data) => {
        if (!controller.signal.aborted) setResponse({ path, result: { status: 'ready', data } })
      },
      (error: unknown) => {
        if (controller.signal.aborted) return
        console.error('[dictionary] lookup failed', path, error)
        setResponse({ path, result: { status: 'error' } })
      },
    )
    return () => controller.abort()
  }, [path, attempt])

  const result: LookupResult<T> = response && response.path === path ? response.result : { status: 'loading' }
  return {
    result,
    retry: () => {
      setResponse(undefined)
      setAttempt((current) => current + 1)
    },
  }
}
