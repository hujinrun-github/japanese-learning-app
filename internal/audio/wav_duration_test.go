package audio_test

import (
	"encoding/binary"
	"testing"
	"time"

	"japanese-learning-app/internal/audio"
)

func TestWAVDuration(t *testing.T) {
	for _, tt := range []struct {
		name    string
		wav     []byte
		want    time.Duration
		wantErr bool
	}{
		{"mono recorder", rawWAV(1, 16000, 16, make([]byte, 16000*2*3)), 3 * time.Second, false},
		{"stereo duration counts frames not samples", rawWAV(2, 24000, 16, make([]byte, 24000*2*2*8)), 8 * time.Second, false},
		{"five minutes", rawWAV(1, 16000, 16, make([]byte, 16000*2*300)), 300 * time.Second, false},
		{"extra metadata does not count as audio", func() []byte {
			wav := rawWAV(1, 16000, 16, make([]byte, 16000*2))
			chunk := make([]byte, 8+1000)
			copy(chunk, "JUNK")
			binary.LittleEndian.PutUint32(chunk[4:8], 1000)
			wav = append(wav, chunk...)
			binary.LittleEndian.PutUint32(wav[4:8], uint32(len(wav)-8))
			return wav
		}(), time.Second, false},
		{"not a WAV", []byte("not a WAV"), 0, true},
		{"truncated data chunk", rawWAV(1, 16000, 16, make([]byte, 16000))[:100], 0, true},
		{"invalid rate", rawWAV(1, 0, 16, make([]byte, 16)), 0, true},
		{"empty audio", rawWAV(1, 16000, 16, nil), 0, true},
		{"unsupported bits", rawWAV(1, 16000, 8, make([]byte, 16)), 0, true},
		{"incomplete PCM frame", rawWAV(2, 16000, 16, make([]byte, 6)), 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := audio.WAVDuration(tt.wav)
			if (err != nil) != tt.wantErr {
				t.Fatalf("WAVDuration() error=%v wantErr=%v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("duration=%s want=%s", got, tt.want)
			}
		})
	}
}
