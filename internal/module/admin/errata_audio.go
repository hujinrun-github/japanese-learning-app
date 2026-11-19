package admin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"japanese-learning-app/internal/audio"
	"japanese-learning-app/internal/cli"
)

// regenerateErrataAudio binds a correction to a target loaded from the report.
// Text is a pronunciation override; the display text still determines the file
// name, so learner playback can find the replacement without changing the lesson.
func (h *Handler) regenerateErrataAudio(w http.ResponseWriter, r *http.Request) {
	if h.cfg.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "内容数据库不可用"})
		return
	}
	id, err := pathID(r)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的反馈编号"})
		return
	}
	var req struct {
		TargetKey string `json:"target_key"`
		Revision  string `json:"revision"`
		regenRequest
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的音频生成请求"})
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Revision == "" || req.TargetKey == "" || req.Text == "" || utf8.RuneCountInString(req.Text) > 30000 || (req.Provider != "vllm" && req.Provider != "sbv") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请选择音频、填写朗读文本及有效的语音服务"})
		return
	}
	if err := h.pendingErrataAudio(r.Context(), id); err != nil {
		h.writeErrataContentError(w, r, err)
		return
	}
	content, err := h.loadErrataContent(r.Context(), id)
	if err != nil {
		h.writeErrataContentError(w, r, err)
		return
	}
	if content.Revision != req.Revision {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "原内容已变化，请重新打开后再生成音频"})
		return
	}
	var target *ErrataAudioTarget
	for i := range content.AudioTargets {
		if content.AudioTargets[i].Key == req.TargetKey {
			target = &content.AudioTargets[i]
			break
		}
	}
	if target == nil || strings.TrimSpace(target.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "音频不属于这条反馈的原内容，或不支持重新生成"})
		return
	}
	kind, targetID, ok := errataAudioTargetID(target.Key)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "该音频不支持重新生成，请修正媒体地址及时间轴"})
		return
	}
	slog.Debug("errata audio synthesis started", "report_id", id, "target", target.Key, "provider", req.Provider, "text_length", utf8.RuneCountInString(req.Text))
	client := cli.NewTTSClient(ttsConfigFromRegenRequest(req.regenRequest))
	data, err := client.Synthesize(r.Context(), req.Text)
	if err != nil {
		slog.Error("errata audio synthesis failed", "report_id", id, "target", target.Key, "err", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "音频生成失败，请检查语音服务配置后重试"})
		return
	}
	parsed, err := audio.ParseWAV(data)
	if err != nil || len(parsed.Samples) == 0 {
		slog.Error("errata TTS returned invalid audio", "report_id", id, "target", target.Key, "err", err, "bytes", len(data))
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "语音服务未返回有效的 PCM WAV 音频，原音频未替换"})
		return
	}
	if kind == "word" {
		data = cli.TrimWAVSilence(data, 0.06)
	}
	directory := "examples"
	if kind == "word" {
		directory = "words"
	}
	filename := fmt.Sprintf("%x", sha256.Sum256([]byte(target.Text)))[:16] + ".wav"
	audioURL := "/audio/" + directory + "/" + filename
	storePath := h.cfg.AudioStorePath
	if storePath == "" {
		storePath = "./data/audio"
	}
	path := filepath.Join(storePath, directory, filename)
	if err := h.saveErrataAudio(r.Context(), id, req.Revision, kind, targetID, path, audioURL, data); err != nil {
		h.writeErrataContentError(w, r, err)
		return
	}
	response := map[string]any{"audio_url": audioURL}
	content, err = h.loadErrataContent(r.Context(), id)
	if err != nil {
		// The correction has committed. Do not report a generation failure when
		// only the follow-up refresh failed; the client can reload the content.
		slog.Error("errata audio saved but content refresh failed", "report_id", id, "target", target.Key, "err", err)
	} else {
		response["content"] = content
	}
	slog.Info("errata audio saved", "report_id", id, "target", target.Key, "audio_url", audioURL)
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) pendingErrataAudio(ctx context.Context, id int64) error {
	var status string
	if err := h.cfg.DB.QueryRowContext(ctx, `SELECT status FROM errata_reports WHERE id=$1`, id).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &errataContentError{status: http.StatusNotFound, message: "反馈不存在"}
		}
		return fmt.Errorf("load errata audio status: %w", err)
	}
	if status != "pending" {
		return &errataContentError{status: http.StatusConflict, message: "请先重新打开反馈，再修正音频"}
	}
	return nil
}

func errataAudioTargetID(key string) (string, int64, bool) {
	kind, rawID, found := strings.Cut(key, ":")
	id, err := strconv.ParseInt(rawID, 10, 64)
	if !found || err != nil || id <= 0 {
		return "", 0, false
	}
	switch kind {
	case "word", "word_example", "grammar_example", "speaking":
		return kind, id, true
	default:
		return "", 0, false
	}
}

func (h *Handler) saveErrataAudio(ctx context.Context, reportID int64, revision, kind string, targetID int64, path, audioURL string, data []byte) error {
	tx, err := h.cfg.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin errata audio transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			slog.Error("rollback errata audio transaction failed", "report_id", reportID, "err", err)
		}
	}()
	// Reports and source rows remain locked through file replacement and commit.
	// The content API takes the same report lock, so edits/resolution cannot race
	// this final check while synthesis itself need not hold database locks.
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM errata_reports WHERE id=$1 FOR UPDATE`, reportID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &errataContentError{status: http.StatusNotFound, message: "反馈已被撤回"}
		}
		return fmt.Errorf("lock errata audio report: %w", err)
	}
	if status != "pending" {
		return &errataContentError{status: http.StatusConflict, message: "反馈状态已变化，请重新打开后再生成"}
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "errata-audio:"+audioURL); err != nil {
		return fmt.Errorf("lock errata audio file: %w", err)
	}
	var update string
	switch kind {
	case "word":
		update = `UPDATE words SET local_audio_url=$2, audio_object_id=NULL, updated_at=now() WHERE id=$1`
	case "word_example":
		update = `UPDATE word_examples SET audio_object_id=NULL WHERE id=$1`
	case "grammar_example":
		update = `UPDATE grammar_examples SET audio_object_id=NULL WHERE id=$1`
	case "speaking":
		update = `UPDATE speaking_materials SET audio_url_legacy=$2, audio_object_id=NULL, updated_at=now() WHERE id=$1`
	default:
		return &errataContentError{status: http.StatusBadRequest, message: "不支持的音频类型"}
	}
	// Use the content editor's lock order (report, root, children) and the
	// transaction's own connection for a consistent final version check.
	current, err := loadErrataContentDocument(ctx, tx, reportID, true)
	if err != nil {
		return fmt.Errorf("reload errata audio content: %w", err)
	}
	if current.content.Revision != revision {
		return &errataContentError{status: http.StatusConflict, message: "生成期间原内容已变化，请重新打开后再生成"}
	}
	args := []any{targetID}
	if kind == "word" || kind == "speaking" {
		args = append(args, audioURL)
	}
	if _, err := tx.ExecContext(ctx, update, args...); err != nil {
		return fmt.Errorf("bind errata replacement audio: %w", err)
	}
	// The parent's update time also reflects an example correction in admin
	// lists and in revisions opened through other reports of this material.
	if kind == "word_example" {
		if _, err := tx.ExecContext(ctx, `UPDATE words SET updated_at=clock_timestamp() WHERE id=(SELECT word_id FROM word_examples WHERE id=$1)`, targetID); err != nil {
			return fmt.Errorf("update corrected word audio revision: %w", err)
		}
	}
	if kind == "grammar_example" {
		if _, err := tx.ExecContext(ctx, `UPDATE grammar_points SET updated_at=clock_timestamp() WHERE id=(SELECT grammar_point_id FROM grammar_examples WHERE id=$1)`, targetID); err != nil {
			return fmt.Errorf("update corrected grammar audio revision: %w", err)
		}
	}
	old, oldErr := os.ReadFile(path)
	if oldErr != nil && !errors.Is(oldErr, os.ErrNotExist) {
		return fmt.Errorf("read existing errata audio: %w", oldErr)
	}
	if err := writeErrataAudioFile(path, data); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		var restoreErr error
		if oldErr == nil {
			restoreErr = writeErrataAudioFile(path, old)
		} else {
			restoreErr = os.Remove(path)
		}
		if restoreErr != nil {
			slog.Error("restore errata audio after commit failure", "report_id", reportID, "path", path, "err", restoreErr)
		}
		return fmt.Errorf("commit errata audio correction: %w", errors.Join(err, restoreErr))
	}
	return nil
}

// A temporary sibling and rename keep readers from observing a partial WAV.
func writeErrataAudioFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create errata audio directory: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".errata-*.wav")
	if err != nil {
		return fmt.Errorf("create errata audio temporary file: %w", err)
	}
	defer func() {
		if err := os.Remove(file.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Error("remove errata audio temporary file failed", "path", file.Name(), "err", err)
		}
	}()
	if _, err := file.Write(data); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			slog.Error("close failed errata audio file", "err", closeErr)
		}
		return fmt.Errorf("write errata audio file: %w", err)
	}
	if err := file.Chmod(0644); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			slog.Error("close failed errata audio file", "err", closeErr)
		}
		return fmt.Errorf("set errata audio permissions: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close errata audio file: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace errata audio file: %w", err)
	}
	return nil
}
