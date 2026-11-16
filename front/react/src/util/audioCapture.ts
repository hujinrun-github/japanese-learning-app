/**
 * Microphone constraints for recordings that will be machine-scored.
 *
 * Browser audio processing must stay OFF for shadowing evaluation:
 *
 * - `autoGainControl` and `noiseSuppression` continuously reshape amplitude
 *   over time. In a real Chrome capture this turned a steady utterance into a
 *   decaying envelope (peak RMS 0.110 -> 0.003), so peak-relative VAD saw only
 *   760 ms of speech out of a 3 s sentence and the backend correctly returned
 *   `too_short`. Sync, fluency and bleed-through detection all read the same
 *   energy envelope, so a time-varying gain destroys the evidence.
 * - `echoCancellation` would strip the reference audio playing from speakers
 *   out of the mic signal. The scorer deliberately detects that as
 *   `bleed_through` and tells the learner to wear headphones; cancelling it
 *   hides the problem instead of fixing it.
 *
 * Verified in a real headless-Chrome recording run: with these constraints the
 * uploaded 16 kHz WAV keeps a flat envelope and the local scorer returns a
 * `scoreable` result.
 */
export function scoringAudioConstraints(): MediaTrackConstraints {
  return {
    echoCancellation: false,
    noiseSuppression: false,
    autoGainControl: false,
  }
}
