import { useCallback, useEffect, useRef, useState } from 'react'
import { scoringAudioConstraints } from '../util/audioCapture'
import {
  WAV_SAMPLE_RATE,
  downmixToMono,
  encodeWAVBlob,
  pickRecorderMimeType,
  wavDurationMS,
} from '../util/wav'

export interface WavRecording {
  /** 16 kHz mono 16-bit PCM WAV, ready to be uploaded as multipart `audio`. */
  blob: Blob
  /** Object URL for local playback. Revoked on the next start / unmount. */
  url: string
  durationMS: number
  sampleRate: number
}

interface WavRecorderState {
  isRecording: boolean
  recording: WavRecording | null
  error: string | null
}

export interface UseWavRecorderResult extends WavRecorderState {
  start: () => Promise<void>
  stop: () => Promise<WavRecording>
  reset: () => void
}

// Records via MediaRecorder, then decodes through an AudioContext pinned to
// 16 kHz. decodeAudioData resamples to the context rate, so the backend always
// receives the same WAV shape regardless of the capture device / browser.
export function useWavRecorder(): UseWavRecorderResult {
  const [state, setState] = useState<WavRecorderState>({
    isRecording: false,
    recording: null,
    error: null,
  })

  const cancelStopRef = useRef<(() => void) | null>(null)
  const generationRef = useRef(0)
  const recorderRef = useRef<MediaRecorder | null>(null)
  const streamRef = useRef<MediaStream | null>(null)
  const chunksRef = useRef<Blob[]>([])
  const urlRef = useRef<string | null>(null)

  const releaseStream = useCallback(() => {
    streamRef.current?.getTracks().forEach((track) => track.stop())
    streamRef.current = null
  }, [])

  const revokeURL = useCallback(() => {
    if (urlRef.current) {
      URL.revokeObjectURL(urlRef.current)
      urlRef.current = null
    }
  }, [])

  useEffect(() => {
    return () => {
      ++generationRef.current
      cancelStopRef.current?.()
      const recorder = recorderRef.current
      if (recorder) {
        recorder.ondataavailable = null
        recorder.onstop = null
        recorder.onerror = null
        if (recorder.state !== 'inactive') recorder.stop()
      }
      revokeURL()
      releaseStream()
    }
  }, [revokeURL, releaseStream])

  const reset = useCallback(() => {
    ++generationRef.current
    cancelStopRef.current?.()
    cancelStopRef.current = null
    const recorder = recorderRef.current
    if (recorder) {
      recorder.ondataavailable = null
      recorder.onstop = null
      recorder.onerror = null
    }
    recorderRef.current = null
    if (recorder && recorder.state !== 'inactive') recorder.stop()
    releaseStream()
    revokeURL()
    setState({ isRecording: false, recording: null, error: null })
  }, [releaseStream, revokeURL])

  const start = useCallback(async () => {
    const generation = ++generationRef.current
    releaseStream()
    try {
      // Keep the raw envelope: browser AGC / noise suppression / echo
      // cancellation would distort the signal the local scorer measures.
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: scoringAudioConstraints(),
      })
      if (generation !== generationRef.current) { stream.getTracks().forEach(track => track.stop()); return }
      streamRef.current = stream

      const mimeType = pickRecorderMimeType((type) => MediaRecorder.isTypeSupported(type))
      if (!mimeType) {
        throw new Error('当前浏览器不支持可录制的音频格式')
      }
      const recorder = new MediaRecorder(stream, { mimeType })
      chunksRef.current = []
      recorder.ondataavailable = (event) => {
        if (generation === generationRef.current && event.data.size > 0) chunksRef.current.push(event.data)
      }
      recorder.start(100)
      recorderRef.current = recorder
      // Keep the previous playback usable if acquiring the microphone fails.
      revokeURL()
      setState({ isRecording: true, recording: null, error: null })
    } catch (err) {
      if (generation !== generationRef.current) throw err
      releaseStream()
      const message = err instanceof Error ? err.message : '无法访问麦克风'
      setState(current => ({ ...current, isRecording: false, error: message }))
      throw err
    }
  }, [revokeURL, releaseStream])

  const stop = useCallback((): Promise<WavRecording> => {
    return new Promise((resolve, reject) => {
      const generation = generationRef.current
      const recorder = recorderRef.current
      if (!recorder) {
        reject(new Error('没有正在进行的录音'))
        return
      }

      cancelStopRef.current = () => reject(new Error('Recording cancelled'))
      recorder.onstop = () => {
        if (generation !== generationRef.current) { reject(new Error('Recording cancelled')); return }
        cancelStopRef.current = null
        void (async () => {
          const mimeType = recorder.mimeType || 'audio/webm'
          const raw = new Blob(chunksRef.current, { type: mimeType })
          recorderRef.current = null
          releaseStream()

          let context: AudioContext | null = null
          try {
            const encoded = await raw.arrayBuffer()
            context = new AudioContext({ sampleRate: WAV_SAMPLE_RATE })
            const decoded = await context.decodeAudioData(encoded)
            const channels: Float32Array[] = []
            for (let i = 0; i < decoded.numberOfChannels; i++) {
              channels.push(decoded.getChannelData(i))
            }
            const mono = downmixToMono(channels)
            if (mono.length === 0) {
              throw new Error('录音里没有有效音频数据')
            }

            if (generation !== generationRef.current) throw new Error('Recording cancelled')
            const wav = encodeWAVBlob(mono, WAV_SAMPLE_RATE)
            const url = URL.createObjectURL(wav)
            urlRef.current = url
            const recording: WavRecording = {
              blob: wav,
              url,
              durationMS: wavDurationMS(mono.length, WAV_SAMPLE_RATE),
              sampleRate: WAV_SAMPLE_RATE,
            }
            setState({ isRecording: false, recording, error: null })
            resolve(recording)
          } catch (err) {
            const message = err instanceof Error ? `无法解码录音：${err.message}` : '无法解码录音'
            if (generation === generationRef.current) setState({ isRecording: false, recording: null, error: message })
            reject(err instanceof Error ? err : new Error(message))
          } finally {
            if (context) {
              try { await context.close() } catch (err) { console.error('[recorder] failed to close audio context', err) }
            }
          }
        })()
      }

      recorder.onerror = () => {
        recorderRef.current = null
        releaseStream()
        const err = new Error('录音过程中发生错误')
        setState({ isRecording: false, recording: null, error: err.message })
        reject(err)
      }

      recorder.stop()
    })
  }, [releaseStream])

  return { ...state, start, stop, reset }
}
