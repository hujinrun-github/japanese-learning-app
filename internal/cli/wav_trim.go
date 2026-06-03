package cli

import (
	"encoding/binary"
	"log/slog"
	"math"
)

// TrimWAVSilence trims leading/trailing silence and breath sounds from WAV word audio.
// Uses windowed RMS + hangover to avoid cutting actual speech.
func TrimWAVSilence(data []byte, threshold float64) []byte {
	if len(data) < 44 {
		return data
	}
	if string(data[0:4]) != "RIFF" {
		return data
	}

	numChannels := int(binary.LittleEndian.Uint16(data[22:24]))
	sampleRate := int(binary.LittleEndian.Uint32(data[24:28]))
	bitsPerSample := int(binary.LittleEndian.Uint16(data[34:36]))
	if bitsPerSample != 16 || numChannels < 1 || numChannels > 2 {
		return data
	}

	// Find "data" chunk
	dataOffset := 36
	for dataOffset < len(data)-8 {
		chunkID := string(data[dataOffset : dataOffset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[dataOffset+4 : dataOffset+8]))
		if chunkID != "data" {
			dataOffset += 8 + chunkSize
			continue
		}

		pcmStart := dataOffset + 8
		pcmEnd := pcmStart + chunkSize
		if pcmEnd > len(data) {
			pcmEnd = len(data)
		}
		pcmData := data[pcmStart:pcmEnd]

		bytesPerSample := bitsPerSample / 8
		bytesPerFrame := numChannels * bytesPerSample
		sampleCount := len(pcmData) / bytesPerFrame
		if sampleCount < 100 {
			return data
		}

		// Read samples as float64
		samples := make([]float64, sampleCount)
		for i := 0; i < sampleCount; i++ {
			val := int16(binary.LittleEndian.Uint16(pcmData[i*bytesPerFrame : i*bytesPerFrame+2]))
			samples[i] = float64(val) / 32768.0
		}

		// Compute RMS per 30ms window with 10ms step (overlapping)
		winSamples := sampleRate * 30 / 1000    // 30ms
		stepSamples := sampleRate * 10 / 1000   // 10ms step
		if winSamples < 80 {
			winSamples = 80
		}
		if stepSamples < 20 {
			stepSamples = 20
		}

		winCount := (sampleCount-winSamples)/stepSamples + 1
		if winCount < 5 {
			return data
		}

		rms := make([]float64, winCount)
		for w := 0; w < winCount; w++ {
			start := w * stepSamples
			sumSq := 0.0
			for i := start; i < start+winSamples && i < sampleCount; i++ {
				sumSq += samples[i] * samples[i]
			}
			rms[w] = math.Sqrt(sumSq / float64(winSamples))
		}

		// Find speech region with hangover:
		// Require N consecutive windows above threshold to mark onset,
		// and N consecutive below threshold to mark offset.
		const hangover = 3 // number of consecutive windows required

		// Find onset
		startWin := 0
		aboveCount := 0
		for i := 0; i < winCount; i++ {
			if rms[i] > threshold {
				aboveCount++
				if aboveCount >= hangover {
					startWin = i - hangover + 1
					break
				}
			} else {
				aboveCount = 0
			}
		}

		// Find offset
		endWin := winCount - 1
		belowCount := 0
		for i := winCount - 1; i > startWin; i-- {
			if rms[i] < threshold {
				belowCount++
				if belowCount >= hangover {
					endWin = i + hangover - 1
					break
				}
			} else {
				belowCount = 0
			}
		}

		if startWin >= endWin || endWin-startWin < 3 {
			return data // too short or all silent
		}

		// Convert to sample indices with padding
		padSamples := winSamples
		startIdx := max(0, startWin*stepSamples-padSamples)
		endIdx := min(sampleCount-1, (endWin+1)*stepSamples+padSamples)

		trimmedSamples := endIdx - startIdx + 1
		if float64(trimmedSamples) >= float64(sampleCount)*0.90 {
			return data // less than 10% trimmed
		}

		// Rebuild WAV
		newPCM := pcmData[startIdx*bytesPerFrame : (endIdx+1)*bytesPerFrame]
		newDataSize := len(newPCM)
		newWAV := make([]byte, 44+newDataSize)
		copy(newWAV[0:44], data[0:44])
		binary.LittleEndian.PutUint32(newWAV[4:8], uint32(36+newDataSize))
		binary.LittleEndian.PutUint32(newWAV[40:44], uint32(newDataSize))
		copy(newWAV[44:], newPCM)

		slog.Debug("TrimWAVSilence done",
			"before", sampleCount,
			"after", trimmedSamples,
		)
		return newWAV
	}
	return data
}
