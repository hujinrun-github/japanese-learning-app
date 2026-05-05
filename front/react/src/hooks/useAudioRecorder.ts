import { useCallback, useRef, useState } from 'react'

interface AudioRecorderState {
  isRecording: boolean
  audioURL: string | null
  error: string | null
}

interface UseAudioRecorderResult extends AudioRecorderState {
  start: () => Promise<void>
  stop: () => Promise<Blob>
}

export function useAudioRecorder(): UseAudioRecorderResult {
  const [state, setState] = useState<AudioRecorderState>({
    isRecording: false,
    audioURL: null,
    error: null,
  })

  const mediaRecorderRef = useRef<MediaRecorder | null>(null)
  const chunksRef = useRef<Blob[]>([])

  const start = useCallback(async () => {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      const mimeType = MediaRecorder.isTypeSupported('audio/webm')
        ? 'audio/webm'
        : 'audio/ogg'
      const recorder = new MediaRecorder(stream, { mimeType })
      chunksRef.current = []

      recorder.ondataavailable = (e) => {
        if (e.data.size > 0) {
          chunksRef.current.push(e.data)
        }
      }

      mediaRecorderRef.current = recorder
      recorder.start(100) // collect in 100ms chunks
      setState({ isRecording: true, audioURL: null, error: null })
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Microphone access denied'
      setState((s) => ({ ...s, error: message }))
      throw err
    }
  }, [])

  const stop = useCallback((): Promise<Blob> => {
    return new Promise((resolve, reject) => {
      const recorder = mediaRecorderRef.current
      if (!recorder) {
        reject(new Error('No active recording'))
        return
      }

      recorder.onstop = () => {
        const mimeType = recorder.mimeType || 'audio/webm'
        const blob = new Blob(chunksRef.current, { type: mimeType })
        const url = URL.createObjectURL(blob)

        // Stop all tracks
        recorder.stream.getTracks().forEach((t) => t.stop())
        mediaRecorderRef.current = null

        setState({ isRecording: false, audioURL: url, error: null })
        resolve(blob)
      }

      recorder.onerror = (e) => {
        reject(e)
      }

      recorder.stop()
    })
  }, [])

  return { ...state, start, stop }
}
