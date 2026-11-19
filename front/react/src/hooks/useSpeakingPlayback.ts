import { useCallback, useEffect, useRef, useState } from 'react'
import { playReferenceAudio, stopCurrent } from '@/util/exampleAudio'
import { estimateSentenceCues, sentenceAtTime } from '@/util/speakingPlayback'

export function useSpeakingPlayback() {
  const [activeSentenceIndex, setActiveSentenceIndex] = useState<number | null>(null)
  const [isPlaying, setIsPlaying] = useState(false)
  const [error, setError] = useState('')
  const request = useRef(0)
  const mounted = useRef(true)

  const stop = useCallback(() => {
    request.current++
    stopCurrent()
    setActiveSentenceIndex(null)
    setIsPlaying(false)
    setError('')
  }, [])

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      request.current++
      stopCurrent()
    }
  }, [])

  const play = useCallback(async (url: string, sentences: string[], sentenceIndex?: number) => {
    const id = ++request.current
    stopCurrent()
    setActiveSentenceIndex(null)
    setIsPlaying(false)
    setError('')
    const cues = estimateSentenceCues(sentences, 1)
    const cue = sentenceIndex === undefined ? undefined : cues[sentenceIndex]
    if (sentenceIndex !== undefined && !cue) return
    setIsPlaying(true)
    try {
      await playReferenceAudio(url, {
        ...(cue ? { startFraction: cue.start, endFraction: cue.end } : {}),
        onProgress: (time, duration) => {
          if (mounted.current && request.current === id && duration > 0 && Number.isFinite(duration)) {
            setActiveSentenceIndex(sentenceAtTime(cues, time / duration))
          }
        },
      })
    } catch (err) {
      if (mounted.current && request.current === id) {
        setError(err instanceof Error ? err.message : 'Reference audio unavailable')
        throw err
      }
    } finally {
      if (mounted.current && request.current === id) {
        setActiveSentenceIndex(null)
        setIsPlaying(false)
      }
    }
  }, [])

  return { activeSentenceIndex, isPlaying, error, play, stop }
}
