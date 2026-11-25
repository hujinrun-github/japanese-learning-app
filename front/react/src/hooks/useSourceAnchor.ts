import { useEffect } from 'react'

export function useSourceAnchor(anchor: string | null, ready: boolean, fallback?: string) {
  useEffect(() => {
    if (!ready || !anchor) return
    const frame = window.requestAnimationFrame(() => {
      const target = document.getElementById(anchor)
        ?? (fallback ? document.getElementById(fallback) : null)
      target?.focus({ preventScroll: true })
      target?.scrollIntoView?.({ block: 'center' })
    })
    return () => window.cancelAnimationFrame(frame)
  }, [anchor, fallback, ready])
}
