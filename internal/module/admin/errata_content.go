package admin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrataContent describes the actual learning content, separate from the report.
type ErrataContent struct {
	Module       string               `json:"module"`
	ItemKey      string               `json:"item_key"`
	Title        string               `json:"title"`
	Revision     string               `json:"revision"`
	Fields       []ErrataContentField `json:"fields"`
	AudioTargets []ErrataAudioTarget  `json:"audio_targets"`
}

type ErrataContentField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Value    string `json:"value"`
	Kind     string `json:"kind"`
	Readonly bool   `json:"readonly,omitempty"`
}

type ErrataAudioTarget struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Text     string `json:"text"`
	AudioURL string `json:"audio_url"`
}

type errataContentError struct {
	status  int
	message string
}

func (e *errataContentError) Error() string { return e.message }
func contentProblem(status int, message string) error {
	return &errataContentError{status: status, message: message}
}

type errataContentQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type errataContentColumn struct {
	name, label, kind string
	readonly          bool
}
type errataContentRecord struct {
	table, prefix, id, version string
	data                       map[string]json.RawMessage
	columns                    []errataContentColumn
}
type errataContentDocument struct {
	content   ErrataContent
	records   []errataContentRecord
	signature []string
}

func (h *Handler) getErrataContent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil || id <= 0 {
		writeJSON(w, 400, map[string]string{"error": "无效的勘误编号"})
		return
	}
	content, err := h.loadErrataContent(r.Context(), id)
	if err != nil {
		h.writeErrataContentError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, content)
}

func (h *Handler) writeErrataContentError(w http.ResponseWriter, r *http.Request, err error) {
	var problem *errataContentError
	if errors.As(err, &problem) {
		writeJSON(w, problem.status, map[string]string{"error": problem.message})
		return
	}
	slog.ErrorContext(r.Context(), "errata content operation failed", "err", err, "report_id", r.PathValue("id"))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "40001" || pgErr.Code == "40P01" {
			writeJSON(w, 409, map[string]string{"error": "内容已被其他操作更新，请重新加载后再试"})
			return
		}
		if strings.HasPrefix(pgErr.Code, "23") || pgErr.Code == "22P02" || pgErr.Code == "P0001" {
			writeJSON(w, 400, map[string]string{"error": "修改不符合内容约束，请检查字段；已练习的翻译原文不能修改"})
			return
		}
	}
	writeJSON(w, 500, map[string]string{"error": "无法读取或保存原内容"})
}

func (h *Handler) loadErrataContent(ctx context.Context, reportID int64) (ErrataContent, error) {
	if h.cfg.DB == nil {
		return ErrataContent{}, contentProblem(503, "内容数据库尚未配置")
	}
	doc, err := loadErrataContentDocument(ctx, h.cfg.DB, reportID, false)
	if err != nil {
		return ErrataContent{}, err
	}
	return doc.content, nil
}

func (h *Handler) updateErrataContent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil || id <= 0 {
		writeJSON(w, 400, map[string]string{"error": "无效的勘误编号"})
		return
	}
	var req struct {
		Revision string            `json:"revision"`
		Values   map[string]string `json:"values"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "修改格式无效"})
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) || req.Revision == "" || len(req.Values) == 0 {
		writeJSON(w, 400, map[string]string{"error": "请提供当前内容版本和待修改字段"})
		return
	}
	if h.cfg.DB == nil {
		h.writeErrataContentError(w, r, contentProblem(503, "内容数据库尚未配置"))
		return
	}
	tx, err := h.cfg.DB.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		h.writeErrataContentError(w, r, fmt.Errorf("begin content correction: %w", err))
		return
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			slog.ErrorContext(r.Context(), "rollback content correction failed", "err", err)
		}
	}()
	doc, err := loadErrataContentDocument(r.Context(), tx, id, true)
	if err != nil {
		h.writeErrataContentError(w, r, err)
		return
	}
	if req.Revision != doc.content.Revision {
		h.writeErrataContentError(w, r, contentProblem(409, "原内容已发生变化，请重新加载后再保存"))
		return
	}
	if err := saveErrataContentValues(r.Context(), tx, &doc, req.Values); err != nil {
		h.writeErrataContentError(w, r, err)
		return
	}
	refreshed, err := loadErrataContentDocument(r.Context(), tx, id, false)
	if err != nil {
		h.writeErrataContentError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		h.writeErrataContentError(w, r, fmt.Errorf("commit content correction: %w", err))
		return
	}
	slog.InfoContext(r.Context(), "errata source content corrected", "report_id", id, "module", doc.content.Module, "field_count", len(req.Values))
	writeJSON(w, http.StatusOK, refreshed.content)
}

// Table, column and predicate names below are internal allowlists, never request input.
func loadErrataContentDocument(ctx context.Context, q errataContentQueryer, reportID int64, lock bool) (errataContentDocument, error) {
	doc := errataContentDocument{content: ErrataContent{Fields: []ErrataContentField{}, AudioTargets: []ErrataAudioTarget{}}}
	query := `SELECT module,item_key,status FROM errata_reports WHERE id=$1`
	if lock {
		query += " FOR UPDATE"
	}
	var status string
	if err := q.QueryRowContext(ctx, query, reportID).Scan(&doc.content.Module, &doc.content.ItemKey, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return doc, contentProblem(404, "勘误报告不存在")
		}
		return doc, fmt.Errorf("load errata report content reference: %w", err)
	}
	if lock && status != "pending" {
		return doc, contentProblem(409, "请先重新打开反馈，再修正原内容")
	}
	doc.signature = []string{doc.content.Module, doc.content.ItemKey}
	parts := strings.Split(doc.content.ItemKey, ":")
	key := parts[0]
	exactExample := len(parts) == 3 && parts[1] == "example"
	if err := validateErrataContentKey(doc.content.Module, parts); err != nil {
		return doc, err
	}
	var root *errataContentRecord
	var err error
	switch doc.content.Module {
	case "word":
		root, err = doc.addOne(ctx, q, "words", "word", "id=$1", []any{key}, lock)
		if err == nil {
			root.columns = contentColumns("kanji_form|单词|text", "reading|假名读音|text", "meaning|中文释义|textarea", "part_of_speech|词性|text", "jlpt_level|JLPT 等级|text", "reading_type|音读或训读|text", "verb_type|自他动词|text")
			doc.content.Title = rawString(root.data, "kanji_form") + " · " + rawString(root.data, "reading")
			err = doc.addExamples(ctx, q, "word", "word_id", parts, lock)
		}
	case "grammar":
		root, err = doc.addOne(ctx, q, "grammar_points", "grammar", "id=$1", []any{key}, lock)
		if err == nil {
			root.columns = contentColumns("name|语法名称|text", "meaning|中文释义|textarea", "conjunction_rule|接续规则|textarea", "usage_note|用法说明|textarea", "jlpt_level|JLPT 等级|text")
			doc.content.Title = rawString(root.data, "name")
			err = doc.addExamples(ctx, q, "grammar", "grammar_point_id", parts, lock)
		}
		if err == nil && !exactExample {
			condition, args := "grammar_point_id=$1", []any{key}
			if len(parts) == 3 {
				condition += " AND id=$2"
				args = append(args, parts[2])
			}
			count := len(doc.records)
			err = doc.addChildren(ctx, q, "grammar_quiz_questions", "grammar_quiz", condition, args, lock)
			if err == nil && len(parts) == 3 && count == len(doc.records) {
				err = contentProblem(404, "该测验题不存在或不属于报告中的语法")
			}
		}
	case "speaking":
		root, err = doc.addOne(ctx, q, "speaking_materials", "speaking", "id=$1", []any{key}, lock)
		if err == nil {
			root.columns = contentColumns("title|材料标题|text", "text|日语原文|textarea", "lines|分行原文（JSON 数组）|json", "jlpt_level|JLPT 等级|text")
			doc.content.Title = rawString(root.data, "title")
		}
	case "translation":
		root, err = doc.addOne(ctx, q, "translation_sentences", "translation", "id=$1", []any{key}, lock)
		if err == nil {
			root.columns = contentColumns("source_text|翻译原文|textarea", "direction|翻译方向（cn2jp / jp2cn）|text", "reference_translation|参考译文|textarea", "topic|主题|text", "jlpt_level|JLPT 等级|text")
			var practiced bool
			if queryErr := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM translation_records WHERE sentence_id=$1)`, key).Scan(&practiced); queryErr != nil {
				return doc, fmt.Errorf("check translation source immutability: %w", queryErr)
			}
			doc.signature = append(doc.signature, strconv.FormatBool(practiced))
			if practiced {
				root.columns[0].readonly = true
				root.columns[1].readonly = true
				root.columns[0].label = "翻译原文（已有练习记录，只读）"
			}
			doc.content.Title = rawString(root.data, "source_text")
		}
	case "writing":
		if _, parseErr := strconv.ParseInt(key, 10, 64); parseErr == nil {
			root, err = doc.addOne(ctx, q, "writing_questions", "writing_question", "id=$1", []any{key}, lock)
			if err == nil {
				root.columns = contentColumns("prompt|写作题目|textarea", "expected_answer|参考答案|textarea", "jlpt_level|JLPT 等级|text")
				doc.content.Title = rawString(root.data, "prompt")
			}
		} else {
			root, err = doc.addOne(ctx, q, "writing_topics", "writing_topic", "id=$1", []any{key}, lock)
			if err == nil {
				root.columns = contentColumns("title|写作主题|text", "essay_prompt|短文题目|textarea", "sentence_prompt|造句题目|textarea", "jlpt_level|JLPT 等级|text")
				doc.content.Title = rawString(root.data, "title")
			}
		}
	case "lesson", "shadowing":
		root, err = doc.addOne(ctx, q, "lessons", "lesson", "id=$1", []any{key}, lock)
		if err == nil {
			root.columns = contentColumns("title|课文标题|text", "jlpt_level|JLPT 等级|text", "audio_url|音频地址（替换后请核查句子时间轴）|text", "video_url|视频地址（替换后请核查句子时间轴）|text")
			if err := prepareLessonContentMedia(ctx, q, root); err != nil {
				return doc, err
			}
			doc.content.Title = rawString(root.data, "title")
			if doc.content.Module == "shadowing" && len(parts) == 4 && parts[1] != "v"+rawString(root.data, "shadowing_version") {
				return doc, contentProblem(409, "报告对应的跟读版本已过期，请先核查当前版本")
			}
			condition, args := "lesson_id=$1", []any{key}
			if len(parts) > 1 {
				condition += " AND position=$2"
				args = append(args, parts[len(parts)-1])
			}
			count := len(doc.records)
			err = doc.addChildren(ctx, q, "lesson_sentences", "lesson_sentence", condition, args, lock)
			if err == nil && len(parts) > 1 && count == len(doc.records) {
				err = contentProblem(404, "报告对应的课文句子不存在")
			}
		}
	}
	if err != nil {
		return doc, err
	}
	if err := doc.buildContent(ctx, q); err != nil {
		return doc, err
	}
	return doc, nil
}

func validateErrataContentKey(module string, parts []string) error {
	valid := false
	example := len(parts) == 3 && parts[1] == "example" && positiveID(parts[2])
	switch module {
	case "word":
		valid = example || len(parts) == 1 || len(parts) == 2 && (parts[1] == "recognition" || parts[1] == "dictation")
	case "grammar":
		valid = example || len(parts) == 1 || len(parts) == 3 && parts[1] == "quiz" && positiveID(parts[2])
	case "speaking", "translation":
		valid = len(parts) == 1
	case "lesson":
		valid = len(parts) == 1 || len(parts) == 3 && parts[1] == "sentence" && nonnegativeID(parts[2])
	case "shadowing":
		valid = len(parts) == 1 || len(parts) == 4 && strings.HasPrefix(parts[1], "v") && positiveID(strings.TrimPrefix(parts[1], "v")) && parts[2] == "sentence" && nonnegativeID(parts[3])
	case "writing":
		valid = len(parts) == 2 && (parts[1] == "essay" || parts[1] == "sentence") && len(parts[0]) > 0 && len(parts[0]) <= 100
		if valid {
			for _, c := range parts[0] {
				if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
					valid = false
					break
				}
			}
		}
	}
	if module != "writing" && !positiveID(parts[0]) {
		valid = false
	}
	if !valid {
		return contentProblem(400, "报告中的内容定位格式无效，请核查模块、编号和练习模式")
	}
	return nil
}
func positiveID(s string) bool {
	id, err := strconv.ParseInt(s, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == s
}
func nonnegativeID(s string) bool {
	id, err := strconv.ParseInt(s, 10, 64)
	return err == nil && id >= 0 && strconv.FormatInt(id, 10) == s
}

func contentColumns(specs ...string) []errataContentColumn {
	cols := make([]errataContentColumn, 0, len(specs))
	for _, spec := range specs {
		parts := strings.Split(spec, "|")
		cols = append(cols, errataContentColumn{name: parts[0], label: parts[1], kind: parts[2]})
	}
	return cols
}

func (doc *errataContentDocument) addOne(ctx context.Context, q errataContentQueryer, table, prefix, predicate string, args []any, lock bool) (*errataContentRecord, error) {
	query := "SELECT to_jsonb(t)::text,t.xmin::text FROM " + table + " t WHERE " + predicate
	if lock {
		query += " FOR UPDATE"
	}
	var raw, version string
	if err := q.QueryRowContext(ctx, query, args...).Scan(&raw, &version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, contentProblem(404, "报告对应的原内容不存在，可能已被删除或更换")
		}
		return nil, fmt.Errorf("load errata content %s: %w", table, err)
	}
	record, err := decodeContentRecord(table, prefix, raw, version)
	if err != nil {
		return nil, err
	}
	doc.records = append(doc.records, record)
	return &doc.records[len(doc.records)-1], nil
}

func decodeContentRecord(table, prefix, raw, version string) (errataContentRecord, error) {
	record := errataContentRecord{table: table, prefix: prefix, version: version}
	if err := json.Unmarshal([]byte(raw), &record.data); err != nil {
		return record, fmt.Errorf("decode errata content %s: %w", table, err)
	}
	record.id = rawString(record.data, "id")
	return record, nil
}

func (doc *errataContentDocument) addExamples(ctx context.Context, q errataContentQueryer, prefix, parentColumn string, parts []string, lock bool) error {
	predicate, args := parentColumn+"=$1", []any{parts[0]}
	exact := len(parts) == 3 && parts[1] == "example"
	if exact {
		predicate += " AND id=$2"
		args = append(args, parts[2])
	}
	count := len(doc.records)
	if err := doc.addChildren(ctx, q, prefix+"_examples", prefix+"_example", predicate, args, lock); err != nil {
		return err
	}
	if exact && len(doc.records) == count {
		return contentProblem(404, "该例句不存在或不属于报告中的条目")
	}
	return nil
}

func (doc *errataContentDocument) addChildren(ctx context.Context, q errataContentQueryer, table, prefix, predicate string, args []any, lock bool) error {
	query := "SELECT to_jsonb(t)::text,t.xmin::text FROM " + table + " t WHERE " + predicate + " ORDER BY position,id"
	if lock {
		query += " FOR UPDATE"
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("load errata child content %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw, version string
		if err := rows.Scan(&raw, &version); err != nil {
			return fmt.Errorf("scan errata child %s: %w", table, err)
		}
		record, err := decodeContentRecord(table, prefix, raw, version)
		if err != nil {
			return err
		}
		switch prefix {
		case "word_example", "grammar_example":
			record.columns = contentColumns("japanese|日语例句|textarea", "chinese|中文译文|textarea", "furigana_html|假名注音（ruby / rt 标签）|textarea")
		case "grammar_quiz":
			record.columns = contentColumns("prompt|题干|textarea", "options|选项（JSON 字符串数组）|json", "answer|正确答案|textarea", "explanation|解析|textarea")
		case "lesson_sentence":
			record.columns = contentColumns("tokens|日文及注音（JSON：surface、reading）|json", "chinese|中文译文|textarea", "start_ms|音频开始时间（毫秒）|text", "end_ms|音频结束时间（毫秒）|text", "paragraph|段落序号（从 0 开始）|text")
		}
		doc.records = append(doc.records, record)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read errata child rows %s: %w", table, err)
	}
	return nil
}

func rawString(data map[string]json.RawMessage, key string) string {
	raw := data[key]
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	if string(raw) == "null" {
		return ""
	}
	return string(raw)
}

func (doc *errataContentDocument) buildContent(ctx context.Context, q errataContentQueryer) error {
	for _, record := range doc.records {
		raw, err := json.Marshal(record.data)
		if err != nil {
			return fmt.Errorf("encode content revision: %w", err)
		}
		doc.signature = append(doc.signature, record.prefix, record.id, record.version, string(raw))
		for _, col := range record.columns {
			label := col.label
			if record.prefix == "word_example" || record.prefix == "grammar_example" || record.prefix == "grammar_quiz" || record.prefix == "lesson_sentence" {
				position, _ := strconv.Atoi(rawString(record.data, "position"))
				label = fmt.Sprintf("第 %d 条 · %s", position+1, label)
			}
			doc.content.Fields = append(doc.content.Fields, ErrataContentField{Key: record.prefix + ":" + record.id + "." + col.name, Label: label, Value: rawString(record.data, col.name), Kind: col.kind, Readonly: col.readonly})
		}
		var text, label, fallback string
		switch record.prefix {
		case "word":
			text = rawString(record.data, "reading")
			label = "单词发音"
			fallback = rawString(record.data, "local_audio_url")
		case "word_example", "grammar_example":
			text = rawString(record.data, "japanese")
			position, _ := strconv.Atoi(rawString(record.data, "position"))
			label = fmt.Sprintf("第 %d 条例句发音", position+1)
		case "speaking":
			text = rawString(record.data, "text")
			label = "口语参考音频"
			fallback = rawString(record.data, "audio_url_legacy")
		default:
			continue
		}
		audioURL := fallback
		switch record.prefix {
		case "word_example", "grammar_example":
			// Learner examples play the canonical text hash. That route serves
			// corrected local files before any migrated object-store copy.
			if text != "" {
				hash := sha256.Sum256([]byte(text))
				audioURL = "/audio/examples/" + hex.EncodeToString(hash[:])[:16] + ".wav"
			}
		case "word":
			if id := rawString(record.data, "audio_object_id"); id != "" {
				var available bool
				if err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM audio_objects WHERE id=$1 AND deleted_at IS NULL)`, id).Scan(&available); err != nil {
					return fmt.Errorf("check content audio: %w", err)
				}
				if available {
					audioURL = "/api/v1/audio/" + id + "/stream"
				}
			}
			// Match getWordAudioURL's normalization of legacy filenames.
			if audioURL != "" && !strings.HasPrefix(audioURL, "/") {
				parsed, err := url.Parse(audioURL)
				if err != nil || parsed.Scheme == "" {
					audioURL = "/audio/words/" + audioURL
				}
			}
		case "speaking":
			// SpeakingStore and scoring use this configured reference verbatim.
			// An unbound object/file is not a learner reference until saved.
		}
		doc.content.AudioTargets = append(doc.content.AudioTargets, ErrataAudioTarget{Key: record.prefix + ":" + record.id, Label: label, Text: text, AudioURL: audioURL})
	}
	signature, err := json.Marshal(doc.signature)
	if err != nil {
		return fmt.Errorf("encode correction revision: %w", err)
	}
	hash := sha256.Sum256(signature)
	doc.content.Revision = hex.EncodeToString(hash[:])
	return nil
}
