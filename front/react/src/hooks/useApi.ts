import { useCallback, useEffect, useRef, useState } from 'react'

interface UseApiState<T> {
  data: T | null
  loading: boolean
  error: Error | null
}

interface UseApiResult<T> extends UseApiState<T> {
  refetch: () => void
}

export function useApi<T>(
  fetcher: (signal: AbortSignal) => Promise<T>,
): UseApiResult<T> {
  const [state, setState] = useState<UseApiState<T>>({
    data: null,
    loading: true,
    error: null,
  })

  const fetcherRef = useRef(fetcher)
  fetcherRef.current = fetcher

  const [tick, setTick] = useState(0)

  const refetch = useCallback(() => {
    setTick((t) => t + 1)
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    setState({ data: null, loading: true, error: null })

    fetcherRef.current(controller.signal)
      .then((data) => {
        if (!controller.signal.aborted) {
          setState({ data, loading: false, error: null })
        }
      })
      .catch((err: unknown) => {
        if (!controller.signal.aborted) {
          setState({ data: null, loading: false, error: err as Error })
        }
      })

    return () => {
      controller.abort()
    }
  }, [tick])

  return { ...state, refetch }
}
