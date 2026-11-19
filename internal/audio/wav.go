package audio

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"time"
)

// WAV holds decoded mono samples in [-1, 1] and the sample rate in Hz.
type WAV struct {
	Samples    []float64
	SampleRate int
}

const wavHeaderSize = 44

// ParseWAV decodes a 16-bit PCM RIFF/WAVE file (mono or stereo) into mono samples.
// Only uncompressed PCM is supported; other formats return an explicit error.
func ParseWAV(data []byte) (WAV, error) {
	slog.Debug("audio.ParseWAV called", "bytes", len(data))
	pcm, channels, sampleRate, err := parsePCM16WAV(data)
	if err != nil {
		slog.Error("audio.ParseWAV: invalid audio", "err", err)
		return WAV{}, fmt.Errorf("audio.ParseWAV: %w", err)
	}

	bytesPerFrame := channels * 2
	frameCount := len(pcm) / bytesPerFrame
	samples := make([]float64, frameCount)
	for i := 0; i < frameCount; i++ {
		base := i * bytesPerFrame
		left := float64(int16(binary.LittleEndian.Uint16(pcm[base : base+2])))
		if channels == 2 {
			right := float64(int16(binary.LittleEndian.Uint16(pcm[base+2 : base+4])))
			samples[i] = (left + right) / (2 * 32768)
		} else {
			samples[i] = left / 32768
		}
	}

	slog.Debug("audio.ParseWAV done", "sample_rate", sampleRate, "channels", channels, "frames", frameCount)
	return WAV{Samples: samples, SampleRate: sampleRate}, nil
}

// WAVDuration validates PCM16 WAV metadata and counts frames without decoding
// or allocating samples. Chunk padding and metadata do not extend the duration.
func WAVDuration(data []byte) (time.Duration, error) {
	pcm, channels, sampleRate, err := parsePCM16WAV(data)
	if err != nil {
		slog.Error("audio.WAVDuration: invalid audio", "err", err)
		return 0, fmt.Errorf("audio.WAVDuration: %w", err)
	}
	frames := len(pcm) / (channels * 2)
	return time.Duration(int64(frames) * int64(time.Second) / int64(sampleRate)), nil
}

func parsePCM16WAV(data []byte) (pcm []byte, channels, sampleRate int, err error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, 0, 0, fmt.Errorf("invalid RIFF/WAVE header (bytes=%d)", len(data))
	}

	var (
		audioFormat   int
		bitsPerSample int
	)

	for off := 12; off+8 <= len(data); {
		chunkID := string(data[off : off+4])
		chunkSize := binary.LittleEndian.Uint32(data[off+4 : off+8])
		body := off + 8
		if uint64(chunkSize) > uint64(len(data)-body) {
			return nil, 0, 0, fmt.Errorf("chunk %q size %d exceeds file length", chunkID, chunkSize)
		}
		end := body + int(chunkSize)

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return nil, 0, 0, fmt.Errorf("fmt chunk too short (%d bytes)", chunkSize)
			}
			audioFormat = int(binary.LittleEndian.Uint16(data[body : body+2]))
			channels = int(binary.LittleEndian.Uint16(data[body+2 : body+4]))
			sampleRate = int(binary.LittleEndian.Uint32(data[body+4 : body+8]))
			bitsPerSample = int(binary.LittleEndian.Uint16(data[body+14 : body+16]))
		case "data":
			pcm = data[body:end]
		}

		off = end + int(chunkSize&1) // chunks are word aligned
	}

	if err := validatePCMFormat(audioFormat, channels, sampleRate, bitsPerSample, len(pcm)); err != nil {
		return nil, 0, 0, fmt.Errorf("validate WAV PCM format: %w", err)
	}
	return pcm, channels, sampleRate, nil
}

func validatePCMFormat(format, channels, rate, bits, pcmLen int) error {
	if format != 1 {
		return fmt.Errorf("unsupported audio format %d (only PCM=1)", format)
	}
	if channels != 1 && channels != 2 {
		return fmt.Errorf("unsupported channel count %d", channels)
	}
	if rate <= 0 {
		return fmt.Errorf("invalid sample rate %d", rate)
	}
	if bits != 16 {
		return fmt.Errorf("unsupported bits per sample %d (only 16)", bits)
	}
	if pcmLen == 0 {
		return fmt.Errorf("missing or empty data chunk")
	}
	if pcmLen%(channels*2) != 0 {
		return fmt.Errorf("incomplete PCM frame (bytes=%d channels=%d)", pcmLen, channels)
	}
	return nil
}

// EncodeWAV writes mono float64 samples as a canonical 16-bit PCM WAV file.
func EncodeWAV(samples []float64, sampleRate int) ([]byte, error) {
	if len(samples) == 0 {
		return nil, fmt.Errorf("audio.EncodeWAV: no samples")
	}
	if sampleRate <= 0 {
		return nil, fmt.Errorf("audio.EncodeWAV: invalid sample rate %d", sampleRate)
	}

	dataSize := len(samples) * 2
	out := make([]byte, wavHeaderSize+dataSize)
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+dataSize))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)
	binary.LittleEndian.PutUint16(out[20:22], 1)
	binary.LittleEndian.PutUint16(out[22:24], 1)
	binary.LittleEndian.PutUint32(out[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(out[28:32], uint32(sampleRate*2))
	binary.LittleEndian.PutUint16(out[32:34], 2)
	binary.LittleEndian.PutUint16(out[34:36], 16)
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(dataSize))

	for i, v := range samples {
		clamped := math.Max(-1, math.Min(1, v))
		quantized := int(math.Round(clamped * 32768))
		if quantized > 32767 {
			quantized = 32767
		}
		if quantized < -32768 {
			quantized = -32768
		}
		binary.LittleEndian.PutUint16(out[wavHeaderSize+i*2:], uint16(int16(quantized)))
	}
	return out, nil
}

// SliceWAV extracts a time range (milliseconds) from a WAV file and returns a
// new canonical mono WAV. The end is clamped to the end of the source audio.
func SliceWAV(data []byte, startMS, endMS int64) ([]byte, error) {
	slog.Debug("audio.SliceWAV called", "start_ms", startMS, "end_ms", endMS, "bytes", len(data))
	parsed, err := ParseWAV(data)
	if err != nil {
		return nil, fmt.Errorf("audio.SliceWAV parse: %w", err)
	}
	if startMS < 0 || endMS <= startMS {
		return nil, fmt.Errorf("audio.SliceWAV: invalid range [%d, %d)", startMS, endMS)
	}

	startIdx := int64(parsed.SampleRate) * startMS / 1000
	endIdx := int64(parsed.SampleRate) * endMS / 1000
	if startIdx >= int64(len(parsed.Samples)) {
		return nil, fmt.Errorf("audio.SliceWAV: start %dms is beyond audio duration", startMS)
	}
	if endIdx > int64(len(parsed.Samples)) {
		endIdx = int64(len(parsed.Samples))
	}

	out, err := EncodeWAV(parsed.Samples[startIdx:endIdx], parsed.SampleRate)
	if err != nil {
		return nil, fmt.Errorf("audio.SliceWAV encode: %w", err)
	}
	slog.Debug("audio.SliceWAV done", "frames", endIdx-startIdx, "bytes", len(out))
	return out, nil
}
