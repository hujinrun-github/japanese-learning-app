package shadowing

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"japanese-learning-app/internal/audio"
	"japanese-learning-app/internal/module/lesson"
)

// ObjectLoader reads an audio object by its database id (MinIO-backed lessons).
type ObjectLoader func(ctx context.Context, objectID int64) ([]byte, error)

// ReferenceAudioReader returns the WAV bytes for a single lesson sentence.
type ReferenceAudioReader interface {
	ReadSentence(ctx context.Context, l *lesson.Lesson, s lesson.Sentence) ([]byte, error)
}

type fsReferenceAudioReader struct {
	audioStorePath string
	loader         ObjectLoader
}

// NewReferenceAudioReader supports two lesson audio sources:
//   - legacy seed lessons: "/audio/lessons/xxx.wav" under audioStorePath
//   - migrated lessons: "/api/v1/audio/{id}/stream" via the injected loader
func NewReferenceAudioReader(audioStorePath string, loader ObjectLoader) ReferenceAudioReader {
	return &fsReferenceAudioReader{audioStorePath: audioStorePath, loader: loader}
}

// ReadAudioURL resolves a full audio URL (local /audio/ path or object-storage
// stream URL) to WAV bytes. Speaking practice reuses the same resolver so its
// materials can point at either storage backend.
func ReadAudioURL(ctx context.Context, audioStorePath string, loader ObjectLoader, audioURL string) ([]byte, error) {
	r := &fsReferenceAudioReader{audioStorePath: audioStorePath, loader: loader}
	return r.readSource(ctx, audioURL)
}

func (r *fsReferenceAudioReader) ReadSentence(ctx context.Context, l *lesson.Lesson, s lesson.Sentence) ([]byte, error) {
	if l == nil {
		return nil, fmt.Errorf("shadowing.ReferenceAudioReader.ReadSentence: nil lesson")
	}
	if s.EndMS <= s.StartMS || s.StartMS < 0 {
		return nil, &Error{Code: ErrShadowingEvalInvalid, Message: "invalid sentence timing"}
	}

	slog.Debug("shadowing.ReferenceAudioReader.ReadSentence called",
		"lesson_id", l.ID, "audio_url", l.AudioURL, "start_ms", s.StartMS, "end_ms", s.EndMS)
	full, err := r.readSource(ctx, l.AudioURL)
	if err != nil {
		return nil, err
	}
	sliced, err := audio.SliceWAV(full, s.StartMS, s.EndMS)
	if err != nil {
		return nil, &Error{
			Code:    ErrShadowingReferenceMissing,
			Message: "failed to extract reference sentence audio",
			Err:     err,
		}
	}
	return sliced, nil
}

func (r *fsReferenceAudioReader) readSource(ctx context.Context, audioURL string) ([]byte, error) {
	audioURL = strings.TrimSpace(audioURL)
	switch {
	case strings.HasPrefix(audioURL, "/api/v1/audio/"):
		parts := strings.Split(strings.Trim(audioURL, "/"), "/")
		if len(parts) < 4 {
			return nil, missingReference(fmt.Errorf("invalid object audio url %q", audioURL))
		}
		objectID, err := strconv.ParseInt(parts[3], 10, 64)
		if err != nil || objectID <= 0 {
			return nil, missingReference(fmt.Errorf("invalid object id in %q: %w", audioURL, err))
		}
		if r.loader == nil {
			return nil, missingReference(fmt.Errorf("object audio %d requires an object loader", objectID))
		}
		data, err := r.loader(ctx, objectID)
		if err != nil {
			return nil, missingReference(fmt.Errorf("load audio object %d: %w", objectID, err))
		}
		return data, nil

	case strings.HasPrefix(audioURL, "/audio/"):
		key := strings.TrimPrefix(audioURL, "/audio/")
		if !fs.ValidPath(key) {
			return nil, missingReference(fmt.Errorf("unsafe audio path %q", key))
		}
		root := r.audioStorePath
		if root == "" {
			return nil, missingReference(fmt.Errorf("audio store path is not configured"))
		}
		file, err := os.OpenInRoot(root, key)
		if err != nil {
			return nil, missingReference(fmt.Errorf("open reference audio %q: %w", key, err))
		}
		defer func() {
			if err := file.Close(); err != nil {
				slog.Error("shadowing.ReferenceAudioReader: close file failed", "err", err, "key", key)
			}
		}()
		data, err := io.ReadAll(file)
		if err != nil {
			return nil, missingReference(fmt.Errorf("read reference audio %q: %w", key, err))
		}
		return data, nil

	default:
		return nil, missingReference(fmt.Errorf("unsupported lesson audio url %q", audioURL))
	}
}

func missingReference(err error) error {
	slog.Error("shadowing.ReferenceAudioReader: reference audio unavailable", "err", err)
	return &Error{
		Code:    ErrShadowingReferenceMissing,
		Message: "reference audio is unavailable for this lesson",
		Err:     err,
	}
}
