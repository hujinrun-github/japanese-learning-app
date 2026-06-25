package cli

import (
	"database/sql"
	"fmt"
	"strings"
)

// verifier runs post-migration verification checks.
type verifier struct {
	sqliteDB *sql.DB
	pgDB     *sql.DB
}

// verifyAll runs all verification checks and returns any issues found.
func (v *verifier) verifyAll() []string {
	var issues []string

	// Row count comparison
	issues = append(issues, v.compareCounts()...)

	// FK integrity
	issues = append(issues, v.checkFKIntegrity()...)

	// Critical type conversion checks
	issues = append(issues, v.checkCriticalConversions()...)

	// JSON split validation
	issues = append(issues, v.checkJSONSplit()...)

	return issues
}

func (v *verifier) compareCounts() []string {
	var issues []string

	// Tables that exist in both databases (not PG-only like audio_objects, video_objects)
	tables := []string{
		"users", "words", "word_records", "word_bookmarks",
		"grammar_points", "grammar_records",
		"lessons",
		"speaking_materials", "speaking_records",
		"writing_questions", "writing_records",
		"translation_sources", "translation_sentences", "translation_records",
		"notes", "note_links",
		"study_sessions", "session_summaries",
		"password_reset_tokens",
	}

	for _, t := range tables {
		sqliteCount, err1 := v.expectedSQLiteCount(t)
		pgCount, err2 := v.count(v.pgDB, t)

		if err1 != nil || err2 != nil {
			if err1 != nil && strings.Contains(err1.Error(), "no such table") {
				continue
			}
			if err2 != nil && strings.Contains(err2.Error(), "does not exist") {
				continue
			}
			issues = append(issues, fmt.Sprintf("count %s: SQLite err=%v, PG err=%v", t, err1, err2))
			continue
		}

		if sqliteCount != pgCount {
			issues = append(issues, fmt.Sprintf("count mismatch: %s (SQLite=%d, PG=%d)", t, sqliteCount, pgCount))
		}
	}

	// word_examples count from JSON vs actual rows
	sqliteExamples, _ := v.sqliteCountJSONArray("words", "examples_json")
	pgExamples, err := v.count(v.pgDB, "word_examples")
	if err == nil && sqliteExamples != pgExamples {
		issues = append(issues, fmt.Sprintf("word_examples: SQLite JSON sum=%d, PG rows=%d", sqliteExamples, pgExamples))
	}

	// grammar_examples
	sqliteGExamples, _ := v.sqliteCountCanonicalGrammarJSONArray("examples_json")
	pgGExamples, err := v.count(v.pgDB, "grammar_examples")
	if err == nil && sqliteGExamples != pgGExamples {
		issues = append(issues, fmt.Sprintf("grammar_examples: SQLite JSON sum=%d, PG rows=%d", sqliteGExamples, pgGExamples))
	}

	// grammar_quiz_questions
	sqliteQuiz, _ := v.sqliteCountCanonicalGrammarJSONArray("quiz_questions_json")
	pgQuiz, err := v.count(v.pgDB, "grammar_quiz_questions")
	if err == nil && sqliteQuiz != pgQuiz {
		issues = append(issues, fmt.Sprintf("grammar_quiz_questions: SQLite JSON sum=%d, PG rows=%d", sqliteQuiz, pgQuiz))
	}

	// lesson_sentences
	sqliteSentences, _ := v.sqliteCountCanonicalLessonJSONArray("content_furigana_json")
	pgSentences, err := v.count(v.pgDB, "lesson_sentences")
	if err == nil && sqliteSentences != pgSentences {
		issues = append(issues, fmt.Sprintf("lesson_sentences: SQLite JSON sum=%d, PG rows=%d", sqliteSentences, pgSentences))
	}

	// lesson_words
	sqliteWords, _ := v.sqliteCountCanonicalLessonJSONArray("word_ids_json")
	pgWords, err := v.count(v.pgDB, "lesson_words")
	if err == nil && sqliteWords != pgWords {
		issues = append(issues, fmt.Sprintf("lesson_words: SQLite JSON sum=%d, PG rows=%d", sqliteWords, pgWords))
	}

	return issues
}

func (v *verifier) checkFKIntegrity() []string {
	var issues []string

	checks := []struct {
		name  string
		query string
	}{
		{"word_records.user_id", `SELECT COUNT(*) FROM word_records wr LEFT JOIN users u ON wr.user_id = u.id WHERE u.id IS NULL`},
		{"word_records.word_id", `SELECT COUNT(*) FROM word_records wr LEFT JOIN words w ON wr.word_id = w.id WHERE w.id IS NULL`},
		{"grammar_records.user_id", `SELECT COUNT(*) FROM grammar_records gr LEFT JOIN users u ON gr.user_id = u.id WHERE u.id IS NULL`},
		{"grammar_records.grammar_point_id", `SELECT COUNT(*) FROM grammar_records gr LEFT JOIN grammar_points gp ON gr.grammar_point_id = gp.id WHERE gp.id IS NULL`},
		{"speaking_records.user_id", `SELECT COUNT(*) FROM speaking_records sr LEFT JOIN users u ON sr.user_id = u.id WHERE u.id IS NULL`},
		{"speaking_records.material_id", `SELECT COUNT(*) FROM speaking_records sr LEFT JOIN speaking_materials sm ON sr.material_id = sm.id WHERE sm.id IS NULL`},
		{"word_examples.word_id", `SELECT COUNT(*) FROM word_examples we LEFT JOIN words w ON we.word_id = w.id WHERE w.id IS NULL`},
		{"lesson_sentences.lesson_id", `SELECT COUNT(*) FROM lesson_sentences ls LEFT JOIN lessons l ON ls.lesson_id = l.id WHERE l.id IS NULL`},
		{"lesson_words.lesson_id", `SELECT COUNT(*) FROM lesson_words lw LEFT JOIN lessons l ON lw.lesson_id = l.id WHERE l.id IS NULL`},
		{"lesson_words.word_id", `SELECT COUNT(*) FROM lesson_words lw LEFT JOIN words w ON lw.word_id = w.id WHERE w.id IS NULL`},
		{"translation_records.sentence_id", `SELECT COUNT(*) FROM translation_records tr LEFT JOIN translation_sentences ts ON tr.sentence_id = ts.id WHERE ts.id IS NULL`},
		{"note_links.note_id", `SELECT COUNT(*) FROM note_links nl LEFT JOIN notes n ON nl.note_id = n.id WHERE n.id IS NULL`},
		{"note_links.target_note_id", `SELECT COUNT(*) FROM note_links nl LEFT JOIN notes n ON nl.target_note_id = n.id WHERE n.id IS NULL`},
	}

	for _, c := range checks {
		var count int
		if err := v.pgDB.QueryRow(c.query).Scan(&count); err != nil {
			issues = append(issues, fmt.Sprintf("FK check %s: query error: %v", c.name, err))
			continue
		}
		if count > 0 {
			issues = append(issues, fmt.Sprintf("FK integrity: %s has %d orphans", c.name, count))
		}
	}

	return issues
}

func (v *verifier) checkCriticalConversions() []string {
	var issues []string

	// writing_questions: grammar_point_id should never be 0
	var zeroGP int
	if err := v.pgDB.QueryRow(`SELECT COUNT(*) FROM writing_questions WHERE grammar_point_id = 0`).Scan(&zeroGP); err == nil && zeroGP > 0 {
		issues = append(issues, fmt.Sprintf("writing_questions: %d rows have grammar_point_id=0 (should be NULL)", zeroGP))
	}

	// word_records: interval_days should not be NULL
	var nullInterval int
	if err := v.pgDB.QueryRow(`SELECT COUNT(*) FROM word_records WHERE interval_days IS NULL`).Scan(&nullInterval); err == nil && nullInterval > 0 {
		issues = append(issues, fmt.Sprintf("word_records: %d rows have NULL interval_days", nullInterval))
	}

	// translation_records: snapshot columns should not be empty
	var emptySnapshot int
	if err := v.pgDB.QueryRow(`SELECT COUNT(*) FROM translation_records WHERE source_text_snapshot = ''`).Scan(&emptySnapshot); err == nil && emptySnapshot > 0 {
		issues = append(issues, fmt.Sprintf("translation_records: %d rows have empty source_text_snapshot", emptySnapshot))
	}

	return issues
}

func (v *verifier) checkJSONSplit() []string {
	var issues []string

	// Check that grammar_quiz_questions.options is valid JSONB
	var badOptions int
	if err := v.pgDB.QueryRow(`SELECT COUNT(*) FROM grammar_quiz_questions WHERE options IS NULL`).Scan(&badOptions); err == nil && badOptions > 0 {
		issues = append(issues, fmt.Sprintf("grammar_quiz_questions: %d rows have NULL options", badOptions))
	}

	// Check that lesson_sentences.tokens is valid JSONB
	var badTokens int
	if err := v.pgDB.QueryRow(`SELECT COUNT(*) FROM lesson_sentences WHERE tokens IS NULL`).Scan(&badTokens); err == nil && badTokens > 0 {
		issues = append(issues, fmt.Sprintf("lesson_sentences: %d rows have NULL tokens", badTokens))
	}

	return issues
}

// count returns the row count for a table.
func (v *verifier) count(db *sql.DB, table string) (int64, error) {
	var count int64
	err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
	return count, err
}

func (v *verifier) expectedSQLiteCount(table string) (int64, error) {
	switch table {
	case "grammar_points":
		return v.countSQLiteQuery(fmt.Sprintf(
			`SELECT COUNT(*) FROM (SELECT name, %s AS normalized_jlpt_level FROM grammar_points GROUP BY name, normalized_jlpt_level)`,
			sqliteNormalizedJLPTExpr("jlpt_level"),
		))
	case "lessons":
		return v.countSQLiteQuery(fmt.Sprintf(
			`SELECT COUNT(*) FROM (SELECT title, %s AS normalized_jlpt_level FROM lessons GROUP BY title, normalized_jlpt_level)`,
			sqliteNormalizedJLPTExpr("jlpt_level"),
		))
	case "word_records":
		return v.countSQLiteQuery(`SELECT COUNT(*) FROM word_records wr JOIN users u ON u.id = wr.user_id`)
	case "word_bookmarks":
		return v.countSQLiteQuery(`SELECT COUNT(*) FROM word_bookmarks wb JOIN users u ON u.id = wb.user_id`)
	case "grammar_records":
		return v.countSQLiteQuery(`SELECT COUNT(*) FROM grammar_records gr JOIN users u ON u.id = gr.user_id`)
	case "speaking_records":
		return v.countSQLiteQuery(`SELECT COUNT(*) FROM speaking_records sr JOIN users u ON u.id = sr.user_id`)
	case "writing_records":
		return v.countSQLiteQuery(`SELECT COUNT(*) FROM writing_records wr JOIN users u ON u.id = wr.user_id`)
	case "translation_records":
		return v.countSQLiteQuery(`SELECT COUNT(*) FROM translation_records tr JOIN users u ON u.id = tr.user_id`)
	case "notes":
		return v.countSQLiteQuery(`SELECT COUNT(*) FROM notes n JOIN users u ON u.id = n.user_id`)
	case "note_links":
		return v.countSQLiteQuery(`SELECT COUNT(*) FROM note_links nl JOIN users u ON u.id = nl.user_id`)
	case "study_sessions":
		return v.countSQLiteQuery(`SELECT COUNT(*) FROM study_sessions ss JOIN users u ON u.id = ss.user_id`)
	case "session_summaries":
		return v.countSQLiteQuery(`SELECT COUNT(*) FROM session_summaries ss JOIN users u ON u.id = ss.user_id`)
	case "password_reset_tokens":
		return v.countSQLiteQuery(`SELECT COUNT(*) FROM password_reset_tokens prt JOIN users u ON u.id = prt.user_id`)
	default:
		return v.count(v.sqliteDB, table)
	}
}

func (v *verifier) countSQLiteQuery(query string, args ...any) (int64, error) {
	var count int64
	err := v.sqliteDB.QueryRow(query, args...).Scan(&count)
	return count, err
}

// sqliteCountJSONArray counts the total number of elements across all JSON arrays in a column.
func (v *verifier) sqliteCountJSONArray(table, column string) (int64, error) {
	return v.sqliteCountJSONArrayQuery(fmt.Sprintf(`SELECT %s FROM %s WHERE %s != '' AND %s != '[]'`, column, table, column, column))
}

func (v *verifier) sqliteCountCanonicalGrammarJSONArray(column string) (int64, error) {
	return v.sqliteCountCanonicalParentJSONArray("grammar_points", "name", column)
}

func (v *verifier) sqliteCountCanonicalLessonJSONArray(column string) (int64, error) {
	return v.sqliteCountCanonicalParentJSONArray("lessons", "title", column)
}

func (v *verifier) sqliteCountCanonicalParentJSONArray(table, keyColumn, jsonColumn string) (int64, error) {
	normalizedLevel := sqliteNormalizedJLPTExpr("jlpt_level")
	return v.sqliteCountJSONArrayQuery(fmt.Sprintf(`
		SELECT src.%s
		FROM %s src
		JOIN (
			SELECT MIN(id) AS id
			FROM %s
			GROUP BY %s, %s
		) canonical ON canonical.id = src.id
		WHERE src.%s != '' AND src.%s != '[]'`,
		jsonColumn, table, table, keyColumn, normalizedLevel, jsonColumn, jsonColumn,
	))
}

func sqliteNormalizedJLPTExpr(column string) string {
	return fmt.Sprintf("CASE WHEN TRIM(%s) IN ('N5','N4','N3','N2','N1') THEN TRIM(%s) ELSE 'N5' END", column, column)
}

func (v *verifier) sqliteCountJSONArrayQuery(query string) (int64, error) {
	rows, err := v.sqliteDB.Query(query)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var total int64
	for rows.Next() {
		var jsonStr string
		if err := rows.Scan(&jsonStr); err != nil {
			return 0, err
		}
		// Count array elements by counting commas between JSON objects
		// This is an approximation: number of top-level objects = 1 + (commas that separate objects)
		if jsonStr == "[]" || jsonStr == "" {
			continue
		}
		// Remove outer brackets
		inner := jsonStr
		if len(inner) >= 2 && inner[0] == '[' {
			inner = inner[1:]
		}
		if len(inner) >= 1 && inner[len(inner)-1] == ']' {
			inner = inner[:len(inner)-1]
		}
		if inner == "" {
			continue
		}
		// Count top-level objects by finding "}," or "]," patterns
		// Simpler approach: count top-level separators
		depth := 0
		count := int64(1) // at least 1 element if we got here
		for _, ch := range inner {
			switch ch {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			case ',':
				if depth == 0 {
					count++
				}
			}
		}
		total += count
	}
	return total, rows.Err()
}
