package admin

import (
	"context"
	"encoding/json"
	"fmt"
)

// Media URLs already live in the lesson's shadowing configuration. These form
// fields edit that configuration without replacing sentence IDs or timings.
func prepareLessonContentMedia(ctx context.Context, q errataContentQueryer, record *errataContentRecord) error {
	var config map[string]json.RawMessage
	if err := json.Unmarshal(record.data["shadowing_config_json"], &config); err != nil {
		return fmt.Errorf("decode lesson media configuration: %w", err)
	}
	audioURL, videoURL := rawString(config, "audio_url"), rawString(config, "video_url")
	mediaType, mediaURL := rawString(config, "media_type"), rawString(config, "media_url")
	if audioURL == "" && mediaType != "video" {
		audioURL = mediaURL
	}
	if videoURL == "" && mediaType == "video" {
		videoURL = mediaURL
	}
	for _, media := range []struct {
		field, table, prefix string
		value                *string
	}{
		{"audio_object_id", "audio_objects", "/api/v1/audio/", &audioURL},
		{"video_object_id", "video_objects", "/api/v1/videos/", &videoURL},
	} {
		if *media.value != "" {
			continue
		}
		id := rawString(record.data, media.field)
		if id == "" {
			continue
		}
		var available bool
		if err := q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+media.table+" WHERE id=$1 AND deleted_at IS NULL)", id).Scan(&available); err != nil {
			return fmt.Errorf("check lesson media object: %w", err)
		}
		if available {
			*media.value = media.prefix + id + "/stream"
		}
	}
	for key, value := range map[string]string{"audio_url": audioURL, "video_url": videoURL} {
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode lesson media URL: %w", err)
		}
		record.data[key] = encoded
	}
	return nil
}

func hasContentChange(changed map[string]string, key string) bool { _, ok := changed[key]; return ok }

func correctedLessonMediaConfig(record errataContentRecord, changed map[string]string) (string, error) {
	var config map[string]json.RawMessage
	if err := json.Unmarshal(record.data["shadowing_config_json"], &config); err != nil {
		return "", fmt.Errorf("decode corrected lesson media: %w", err)
	}
	if config == nil {
		config = map[string]json.RawMessage{}
	}
	audioURL, videoURL := rawString(record.data, "audio_url"), rawString(record.data, "video_url")
	if next, ok := changed["audio_url"]; ok {
		audioURL = next
	}
	if next, ok := changed["video_url"]; ok {
		videoURL = next
	}
	mediaType, mediaURL := "audio", audioURL
	if videoURL != "" {
		mediaType, mediaURL = "video", videoURL
	}
	for key, value := range map[string]string{"audio_url": audioURL, "video_url": videoURL, "media_type": mediaType, "media_url": mediaURL} {
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("encode corrected lesson media value: %w", err)
		}
		config[key] = encoded
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("encode corrected lesson media configuration: %w", err)
	}
	return string(encoded), nil
}
