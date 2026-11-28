import { useEffect, useRef, type KeyboardEvent, type PointerEvent, type RefObject } from 'react'

type Position = { left: number; top: number }
type Size = { width: number; height: number }

export function clampFloatingSearchPosition(panel: Position & Size, viewport: Size): Position {
  const gap = 8
  return {
    left: Math.max(gap, Math.min(panel.left, viewport.width - panel.width - gap)),
    top: Math.max(gap, Math.min(panel.top, viewport.height - panel.height - gap)),
  }
}

function positionPanel(panel: HTMLDialogElement, bounds: DOMRect, left: number, top: number) {
  const viewport = document.documentElement
  const position = clampFloatingSearchPosition(
    { left, top, width: bounds.width, height: bounds.height },
    { width: viewport.clientWidth, height: viewport.clientHeight },
  )
  panel.style.left = `${position.left}px`
  panel.style.top = `${position.top}px`
  panel.style.transform = 'none'
}

export function useFloatingSearchPosition(dialog: RefObject<HTMLDialogElement>) {
  const drag = useRef<{ pointerId: number; offsetX: number; offsetY: number } | null>(null)

  useEffect(() => {
    const panel = dialog.current
    if (!panel) return

    function keepInView() {
      if (!panel?.open || !panel.style.left) return
      const bounds = panel.getBoundingClientRect()
      positionPanel(panel, bounds, bounds.left, bounds.top)
    }

    const observer = new ResizeObserver(keepInView)
    observer.observe(panel)
    window.addEventListener('resize', keepInView)
    return () => {
      observer.disconnect()
      window.removeEventListener('resize', keepInView)
    }
  }, [dialog])

  function onPointerDown(event: PointerEvent<HTMLDivElement>) {
    const panel = dialog.current
    if (!panel || event.button !== 0 || drag.current) return
    const bounds = panel.getBoundingClientRect()
    drag.current = {
      pointerId: event.pointerId,
      offsetX: event.clientX - bounds.left,
      offsetY: event.clientY - bounds.top,
    }
    event.currentTarget.setPointerCapture(event.pointerId)
  }

  function onPointerMove(event: PointerEvent<HTMLDivElement>) {
    const panel = dialog.current
    const active = drag.current
    if (!panel || active?.pointerId !== event.pointerId) return
    positionPanel(panel, panel.getBoundingClientRect(), event.clientX - active.offsetX, event.clientY - active.offsetY)
  }

  function onLostPointerCapture(event: PointerEvent<HTMLDivElement>) {
    if (drag.current?.pointerId === event.pointerId) drag.current = null
  }

  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    const panel = dialog.current
    if (!panel) return
    const step = 24
    const horizontal = event.key === 'ArrowLeft' ? -step : event.key === 'ArrowRight' ? step : 0
    const vertical = event.key === 'ArrowUp' ? -step : event.key === 'ArrowDown' ? step : 0
    if (!horizontal && !vertical) return
    event.preventDefault()
    const bounds = panel.getBoundingClientRect()
    positionPanel(panel, bounds, bounds.left + horizontal, bounds.top + vertical)
  }

  return { onPointerDown, onPointerMove, onLostPointerCapture, onKeyDown }
}
