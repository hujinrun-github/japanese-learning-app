package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"japanese-learning-app/internal/module/lesson"
)

func saveErrataContentValues(ctx context.Context, tx *sql.Tx, doc *errataContentDocument, values map[string]string) error {
	allowed := make(map[string]ErrataContentField, len(doc.content.Fields))
	for _, field := range doc.content.Fields {
		allowed[field.Key] = field
	}
	for key, value := range values {
		field, ok := allowed[key]
		if !ok {
			return contentProblem(400, "修改包含不属于当前内容的字段："+key)
		}
		if field.Readonly && field.Value != value {
			return contentProblem(400, field.Label+"不能修改")
		}
		if len(value) > 256<<10 {
			return contentProblem(400, field.Label+"内容过长")
		}
	}
	// Validate every proposed row before executing the first update. The transaction
	// also protects against constraints rejected by PostgreSQL later in the save.
	changes := make([]map[string]string, len(doc.records))
	for i, record := range doc.records {
		changed := map[string]string{}
		for _, column := range record.columns {
			key := record.prefix + ":" + record.id + "." + column.name
			if value, ok := values[key]; ok && !column.readonly && value != rawString(record.data, column.name) {
				changed[column.name] = value
			}
		}
		if len(changed) == 0 {
			continue
		}
		if record.prefix == "speaking" {
			if text, ok := changed["text"]; ok {
				if _, linesChanged := changed["lines"]; !linesChanged {
					lines := strings.Split(text, "\n")
					encoded, err := json.Marshal(lines)
					if err != nil {
						return fmt.Errorf("encode speaking correction lines: %w", err)
					}
					changed["lines"] = string(encoded)
				}
			}
		}
		if record.prefix == "word_example" || record.prefix == "grammar_example" {
			if _, changedJapanese := changed["japanese"]; changedJapanese {
				if _, changedFurigana := changed["furigana_html"]; !changedFurigana {
					changed["furigana_html"] = ""
				}
			}
		}
		if err := validateErrataContentValues(record, changed); err != nil {
			return err
		}
		changes[i] = changed
	}
	for i, record := range doc.records {
		changed := changes[i]
		if len(changed) == 0 {
			continue
		}
		assignments := []string{}
		args := []any{}
		if record.prefix == "lesson" {
			if hasContentChange(changed, "audio_url") || hasContentChange(changed, "video_url") {
				config, err := correctedLessonMediaConfig(record, changed)
				if err != nil {
					return err
				}
				args = append(args, config)
				assignments = append(assignments, "shadowing_config_json=$1::jsonb")
			}
		}
		// Iterate in schema order for deterministic updates and parameter bindings.
		for _, column := range record.columns {
			if record.prefix == "lesson" && (column.name == "audio_url" || column.name == "video_url") {
				continue
			}
			value, ok := changed[column.name]
			if !ok {
				continue
			}
			args = append(args, value)
			parameter := fmt.Sprintf("$%d", len(args))
			if column.kind == "json" {
				parameter += "::jsonb"
				if column.name == "lines" {
					parameter = "ARRAY(SELECT jsonb_array_elements_text(" + parameter + "))"
				}
			}
			assignments = append(assignments, column.name+"="+parameter)
		}
		if _, ok := record.data["updated_at"]; ok {
			assignments = append(assignments, "updated_at=clock_timestamp()")
		}
		args = append(args, record.id)
		query := "UPDATE " + record.table + " SET " + strings.Join(assignments, ",") + fmt.Sprintf(" WHERE id=$%d", len(args))
		result, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("save corrected %s: %w", record.prefix, err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("count corrected %s: %w", record.prefix, err)
		}
		if count != 1 {
			return contentProblem(409, "原内容已被其他操作删除，请重新加载")
		}
		// Child edits must also invalidate revisions loaded through another report
		// for this parent, without recreating IDs or detaching existing audio.
		if record.prefix == "word_example" {
			if _, err := tx.ExecContext(ctx, `UPDATE words SET updated_at=clock_timestamp() WHERE id=$1`, rawString(record.data, "word_id")); err != nil {
				return fmt.Errorf("update corrected word revision: %w", err)
			}
		}
		if record.prefix == "grammar_example" || record.prefix == "grammar_quiz" {
			if _, err := tx.ExecContext(ctx, `UPDATE grammar_points SET updated_at=clock_timestamp() WHERE id=$1`, rawString(record.data, "grammar_point_id")); err != nil {
				return fmt.Errorf("update corrected grammar revision: %w", err)
			}
		}
		if record.prefix == "lesson_sentence" {
			if _, err := tx.ExecContext(ctx, `UPDATE lessons SET updated_at=clock_timestamp(), char_count=(SELECT COALESCE(sum(char_length(token->>'surface')),0) FROM lesson_sentences s CROSS JOIN LATERAL jsonb_array_elements(s.tokens) token WHERE s.lesson_id=$1) WHERE id=$1`, rawString(record.data, "lesson_id")); err != nil {
				return fmt.Errorf("update corrected lesson revision: %w", err)
			}
		}
	}
	return nil
}

func validateErrataContentValues(record errataContentRecord, changed map[string]string) error {
	value := func(name string) string {
		if next, ok := changed[name]; ok {
			return next
		}
		return rawString(record.data, name)
	}
	for name, next := range changed {
		if name == "jlpt_level" && !(record.prefix == "translation" && next == "") {
			if next != "N5" && next != "N4" && next != "N3" && next != "N2" && next != "N1" {
				return contentProblem(400, "JLPT 等级须为 N5、N4、N3、N2 或 N1")
			}
		}
		switch name {
		case "audio_url", "video_url":
			if next != "" {
				parsed, err := url.Parse(next)
				if err != nil || parsed.User != nil || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https" && !(parsed.Scheme == "" && parsed.Host == "" && strings.HasPrefix(parsed.Path, "/") && !strings.HasPrefix(next, "//"))) || ((parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host == "") {
					return contentProblem(400, "媒体地址须为站内绝对路径或 http(s) 地址")
				}
			}
		case "kanji_form", "reading", "meaning", "name", "japanese", "chinese", "prompt", "answer", "source_text", "essay_prompt", "sentence_prompt", "expected_answer":
			if strings.TrimSpace(next) == "" {
				return contentProblem(400, "必填内容不能为空："+name)
			}
		case "title":
			if record.prefix != "speaking" && strings.TrimSpace(next) == "" {
				return contentProblem(400, "标题不能为空")
			}
		case "text":
			if strings.TrimSpace(next) == "" {
				return contentProblem(400, "日语原文不能为空")
			}
		case "direction":
			if next != "cn2jp" && next != "jp2cn" {
				return contentProblem(400, "翻译方向须为 cn2jp 或 jp2cn")
			}
		case "verb_type":
			if next != "" && next != "自动词" && next != "他动词" && next != "自他两用" {
				return contentProblem(400, "自他动词标签无效")
			}
		case "options", "lines":
			var entries []string
			if json.Unmarshal([]byte(next), &entries) != nil || strings.TrimSpace(next) == "null" {
				return contentProblem(400, "选项或分行内容必须是 JSON 字符串数组")
			}
		case "tokens":
			var tokens []lesson.FuriganaToken
			decoder := json.NewDecoder(strings.NewReader(next))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&tokens) != nil || len(tokens) == 0 {
				return contentProblem(400, "课文标注须为非空 JSON 数组，每项含 surface 和 reading")
			}
			for _, token := range tokens {
				if token.Surface == "" {
					return contentProblem(400, "课文标注的 surface 不能为空")
				}
			}
		case "start_ms", "end_ms", "paragraph":
			if !nonnegativeID(next) {
				return contentProblem(400, "时间和段落序号必须是非负整数")
			}
		case "furigana_html":
			// Learning pages render this field as HTML. Permit only plain ruby
			// markup; attributes, scripts and other tags are never needed here.
			markup := regexp.MustCompile(`</?(ruby|rt|rp)>`).ReplaceAllString(next, "")
			if strings.ContainsAny(markup, "<>") {
				return contentProblem(400, "注音只支持无属性的 ruby、rt、rp 标签")
			}
		}
	}
	if record.prefix == "lesson_sentence" {
		start, err := strconv.ParseInt(value("start_ms"), 10, 64)
		if err != nil {
			return contentProblem(400, "音频开始时间无效")
		}
		end, err := strconv.ParseInt(value("end_ms"), 10, 64)
		if err != nil || end < start {
			return contentProblem(400, "音频结束时间不能早于开始时间")
		}
	}
	if record.prefix == "grammar_quiz" && rawString(record.data, "type") == "multi_choice" {
		var options []string
		if err := json.Unmarshal([]byte(value("options")), &options); err != nil {
			return contentProblem(400, "选择题选项格式无效")
		}
		found := false
		for _, option := range options {
			if option == value("answer") {
				found = true
				break
			}
		}
		if !found {
			return contentProblem(400, "选择题正确答案必须包含在选项中")
		}
	}
	return nil
}
