package speaking_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"japanese-learning-app/internal/module/speaking"
)

func contextTestWAV(rate, channels int, duration time.Duration) []byte {
	frames := int(int64(rate) * int64(duration) / int64(time.Second))
	wav := make([]byte, 44+frames*channels*2)
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:8], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1)
	binary.LittleEndian.PutUint16(wav[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(wav[24:28], uint32(rate))
	binary.LittleEndian.PutUint32(wav[28:32], uint32(rate*channels*2))
	binary.LittleEndian.PutUint16(wav[32:34], uint16(channels*2))
	binary.LittleEndian.PutUint16(wav[34:36], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:44], uint32(len(wav)-44))
	return wav
}

func TestWhisperCPPScorerRequestSettings(t *testing.T) {
	for _, tt := range []struct {
		name       string
		whisperCPP bool
		duration   time.Duration
		rate       int
		channels   int
		invalid    string
		wantCtx    int
	}{
		{"default OpenAI endpoint named inference receives no extensions", false, 3153 * time.Millisecond, 16000, 1, "", 0},
		{"default OpenAI long request receives no extensions", false, 300 * time.Second, 16000, 1, "", 0},
		{"short tested context", true, 3153 * time.Millisecond, 16000, 1, "", 384},
		{"minimum short context", true, time.Second, 16000, 1, "", 384},
		{"eight seconds", true, 8 * time.Second, 16000, 1, "", 512},
		{"sixteen seconds", true, 16 * time.Second, 16000, 1, "", 1024},
		{"twenty four seconds", true, 24 * time.Second, 16000, 1, "", 1408},
		{"long boundary keeps model context", true, 25 * time.Second, 16000, 1, "", 0},
		{"ninety seconds are not shortened", true, 90 * time.Second, 16000, 1, "", 0},
		{"five minutes are not shortened", true, 300 * time.Second, 16000, 1, "", 0},
		{"different sample rate and stereo use real duration", true, 3153 * time.Millisecond, 24000, 2, "", 384},
		{"invalid WAV does not guess context", true, time.Second, 16000, 1, "truncated", 0},
		{"non WAV does not guess context", true, time.Second, 16000, 1, "non-wav", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wav := contextTestWAV(tt.rate, tt.channels, tt.duration)
			switch tt.invalid {
			case "truncated":
				wav = wav[:100] // Keep the declared data length to detect truncation.
			case "non-wav":
				wav = []byte("webm or other unknown container")
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/inference" {
					t.Errorf("path=%s want /inference", r.URL.Path)
				}
				if err := r.ParseMultipartForm(12 << 20); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				defer func() {
					if err := r.MultipartForm.RemoveAll(); err != nil {
						t.Error(err)
					}
				}()
				if r.FormValue("language") != "ja" || r.FormValue("response_format") != "json" || r.FormValue("model") != "base" {
					t.Errorf("standard fields changed: %v", r.Form)
				}
				for key, value := range map[string]string{"best_of": "1", "beam_size": "1", "temperature": "0", "temperature_inc": "0", "token_timestamps": "false"} {
					got := r.FormValue(key)
					if !tt.whisperCPP {
						value = ""
					}
					if got != value {
						t.Errorf("%s=%q want=%q", key, got, value)
					}
				}
				wantCtx := ""
				if tt.wantCtx != 0 {
					wantCtx = strconv.Itoa(tt.wantCtx)
				}
				if got := r.FormValue("audio_ctx"); got != wantCtx {
					t.Errorf("audio_ctx=%q want=%q", got, wantCtx)
				}
				if r.FormValue("duration") != "" || r.FormValue("offset_t") != "" {
					t.Error("must upload and transcribe the entire recording")
				}
				if r.FormValue("no_timestamps") != "" {
					t.Error("must preserve segment timestamps and long-recording seek behavior")
				}
				file, _, err := r.FormFile("file")
				if err != nil {
					t.Error(err)
					return
				}
				defer func() {
					if err := file.Close(); err != nil {
						t.Error(err)
					}
				}()
				uploaded, err := io.ReadAll(file)
				if err != nil {
					t.Error(err)
				}
				if !bytes.Equal(uploaded, wav) {
					t.Errorf("recording was changed or truncated: uploaded=%d original=%d", len(uploaded), len(wav))
				}
				if _, err := w.Write([]byte(`{"text":"駅はどこですか。学校へ行きます。"}`)); err != nil {
					t.Error(err)
				}
			}))
			defer srv.Close()
			scorer := speaking.NewVLLMScorer(srv.URL+"/inference", "base", time.Second)
			if tt.whisperCPP {
				scorer = speaking.NewWhisperCPPScorer(srv.URL+"/inference", "base", time.Second)
			}
			if _, _, err := scorer.ScoreAudio(context.Background(), wav, "駅はどこですか。学校へ行きます。"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestWhisperCPPScorerCancellation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		cancel bool
		cause  error
	}{
		{"client timeout", false, context.DeadlineExceeded},
		{"request cancelled while server processes", true, context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			started := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, err := io.Copy(io.Discard, r.Body); err != nil {
					t.Error(err)
					return
				}
				close(started)
				select {
				case <-r.Context().Done():
				case <-time.After(3 * time.Second):
					t.Error("request did not cancel")
				}
			}))
			defer srv.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			clientTimeout := 50 * time.Millisecond
			if tt.cancel {
				clientTimeout = 3 * time.Second
				go func() {
					<-started
					cancel()
				}()
			}
			_, _, err := speaking.NewWhisperCPPScorer(srv.URL, "base", clientTimeout).ScoreAudio(ctx, contextTestWAV(16000, 1, time.Second), "駅です。")
			if !errors.Is(err, tt.cause) {
				t.Fatalf("error=%v want wrapped %v", err, tt.cause)
			}
		})
	}
}
