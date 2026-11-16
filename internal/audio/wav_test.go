package audio_test

import (
	"encoding/binary"
	"math"
	"testing"

	"japanese-learning-app/internal/audio"
)

func rawWAV(channels, rate, bits int, pcm []byte) []byte {
	b := make([]byte, 44+len(pcm))
	copy(b[0:4], "RIFF")
	binary.LittleEndian.PutUint32(b[4:8], uint32(36+len(pcm)))
	copy(b[8:12], "WAVE")
	copy(b[12:16], "fmt ")
	binary.LittleEndian.PutUint32(b[16:20], 16)
	binary.LittleEndian.PutUint16(b[20:22], 1)
	binary.LittleEndian.PutUint16(b[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(b[24:28], uint32(rate))
	binary.LittleEndian.PutUint32(b[28:32], uint32(rate*channels*bits/8))
	binary.LittleEndian.PutUint16(b[32:34], uint16(channels*bits/8))
	binary.LittleEndian.PutUint16(b[34:36], uint16(bits))
	copy(b[36:40], "data")
	binary.LittleEndian.PutUint32(b[40:44], uint32(len(pcm)))
	copy(b[44:], pcm)
	return b
}

func pcm16(values ...int16) []byte {
	b := make([]byte, len(values)*2)
	for i, v := range values {
		binary.LittleEndian.PutUint16(b[i*2:], uint16(v))
	}
	return b
}

func sineWAV(rate, seconds int, freq float64) []byte {
	n := rate * seconds
	buf := make([]byte, n*2)
	for i := 0; i < n; i++ {
		v := int16(16000 * math.Sin(2*math.Pi*freq*float64(i)/float64(rate)))
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(v))
	}
	return rawWAV(1, rate, 16, buf)
}

func TestParseWAV(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		wantRate    int
		wantSamples int
		wantFirst   float64
		wantErr     bool
	}{
		{
			name:        "mono 16 bit",
			data:        rawWAV(1, 16000, 16, pcm16(0, 16384, -16384, 32767)),
			wantRate:    16000,
			wantSamples: 4,
			wantFirst:   0,
		},
		{
			name:        "stereo downmixed to mono",
			data:        rawWAV(2, 16000, 16, pcm16(1000, 3000, -1000, -3000)),
			wantRate:    16000,
			wantSamples: 2,
			wantFirst:   float64(2000) / 32768,
		},
		{
			name:    "bad riff magic",
			data:    func() []byte { b := rawWAV(1, 16000, 16, pcm16(1)); b[0] = 'X'; return b }(),
			wantErr: true,
		},
		{
			name:    "8 bit unsupported",
			data:    rawWAV(1, 16000, 8, []byte{1, 2, 3, 4}),
			wantErr: true,
		},
		{
			name: "missing data chunk",
			data: func() []byte {
				b := rawWAV(1, 16000, 16, pcm16(1, 2))
				copy(b[36:40], "junk")
				return b
			}(),
			wantErr: true,
		},
		{
			name:    "truncated header",
			data:    []byte("RIFF"),
			wantErr: true,
		},
		{
			name:    "unsupported channel count",
			data:    rawWAV(3, 16000, 16, pcm16(1, 2, 3)),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := audio.ParseWAV(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseWAV() err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.SampleRate != tt.wantRate {
				t.Errorf("SampleRate = %d, want %d", got.SampleRate, tt.wantRate)
			}
			if len(got.Samples) != tt.wantSamples {
				t.Fatalf("len(Samples) = %d, want %d", len(got.Samples), tt.wantSamples)
			}
			if math.Abs(got.Samples[0]-tt.wantFirst) > 1e-6 {
				t.Errorf("Samples[0] = %v, want %v", got.Samples[0], tt.wantFirst)
			}
		})
	}
}

func TestEncodeWAVRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		samples []float64
		rate    int
		wantErr bool
	}{
		{name: "typical", samples: []float64{0, 0.5, -0.5, 0.999}, rate: 16000},
		{name: "clamped", samples: []float64{2.0, -2.0}, rate: 16000},
		{name: "empty", samples: nil, rate: 16000, wantErr: true},
		{name: "zero rate", samples: []float64{0}, rate: 0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := audio.EncodeWAV(tt.samples, tt.rate)
			if (err != nil) != tt.wantErr {
				t.Fatalf("EncodeWAV() err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			got, err := audio.ParseWAV(encoded)
			if err != nil {
				t.Fatalf("ParseWAV(EncodeWAV()): %v", err)
			}
			if got.SampleRate != tt.rate || len(got.Samples) != len(tt.samples) {
				t.Fatalf("round trip mismatch: rate=%d samples=%d", got.SampleRate, len(got.Samples))
			}
			for i := range tt.samples {
				want := math.Max(-1, math.Min(1, tt.samples[i]))
				if math.Abs(got.Samples[i]-want) > 1e-4 {
					t.Errorf("Samples[%d] = %v, want ~%v", i, got.Samples[i], want)
				}
			}
		})
	}
}

func TestSliceWAV(t *testing.T) {
	data := sineWAV(16000, 3, 120)
	tests := []struct {
		name        string
		startMS     int64
		endMS       int64
		wantSamples int
		wantErr     bool
	}{
		{name: "middle second", startMS: 1000, endMS: 2000, wantSamples: 16000},
		{name: "from zero", startMS: 0, endMS: 500, wantSamples: 8000},
		{name: "clamped end", startMS: 2500, endMS: 99999, wantSamples: 8000},
		{name: "invalid range", startMS: 2000, endMS: 1000, wantErr: true},
		{name: "negative start", startMS: -1, endMS: 100, wantErr: true},
		{name: "start beyond audio", startMS: 99999, endMS: 100000, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := audio.SliceWAV(data, tt.startMS, tt.endMS)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SliceWAV() err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			parsed, err := audio.ParseWAV(got)
			if err != nil {
				t.Fatalf("ParseWAV(SliceWAV()): %v", err)
			}
			if len(parsed.Samples) != tt.wantSamples {
				t.Errorf("len(Samples) = %d, want %d", len(parsed.Samples), tt.wantSamples)
			}
		})
	}
}
