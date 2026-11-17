package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"japanese-learning-app/internal/module/word"
)

const reviewWordColumnsSQLite = `w.id, w.kanji_form, w.reading, w.part_of_speech, w.meaning,
       w.examples_json, w.jlpt_level, w.reading_type, w.audio_url`

// ListReviewCandidates reads overdue and new words with the limits pushed down
// to SQL. SQLite stores examples inline, so two queries are enough; keeping
// the same contract as the PostgreSQL store means the service has one code
// path regardless of the runtime.
func (s *WordStore) ListReviewCandidates(userID int64, level word.JLPTLevel, dueLimit, newLimit int) ([]word.ReviewCandidate, error) {
	slog.Debug("WordStore.ListReviewCandidates called", "user_id", userID, "level", level, "due_limit", dueLimit, "new_limit", newLimit)

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if dueLimit < 1 {
		dueLimit = 1
	}
	if newLimit < 0 {
		newLimit = 0
	}

	dueRows, err := s.db.Query(
		`SELECT `+reviewWordColumnsSQLite+`,
		        r.id, r.user_id, r.word_id, r.mastery_level, r.next_review_at,
		        r.ease_factor, r.interval, r.review_history_json, r.updated_at
		   FROM word_records r
		   JOIN words w ON w.id = r.word_id
		  WHERE r.user_id = ?
		    AND w.jlpt_level = ?
		    AND r.next_review_at <= datetime('now')
		    AND (r.updated_at IS NULL OR r.updated_at < ?)
		  ORDER BY r.next_review_at ASC, w.id ASC
		  LIMIT ?`,
		userID, level, formatSQLiteTime(todayStart), dueLimit,
	)
	if err != nil {
		slog.Error("WordStore.ListReviewCandidates: due query failed", "err", err, "user_id", userID, "level", level)
		return nil, fmt.Errorf("data.WordStore.ListReviewCandidates due query: %w", err)
	}
	due, err := scanReviewCandidatesSQLite(dueRows, true)
	if closeErr := dueRows.Close(); closeErr != nil {
		slog.Error("WordStore.ListReviewCandidates: close due rows failed", "err", closeErr)
	}
	if err != nil {
		slog.Error("WordStore.ListReviewCandidates: scan due rows failed", "err", err)
		return nil, err
	}

	newRows, err := s.db.Query(
		`SELECT `+reviewWordColumnsSQLite+`
		   FROM words w
		   LEFT JOIN word_records r ON r.word_id = w.id AND r.user_id = ?
		  WHERE w.jlpt_level = ?
		    AND r.id IS NULL
		  ORDER BY w.id ASC
		  LIMIT ?`,
		userID, level, newLimit,
	)
	if err != nil {
		slog.Error("WordStore.ListReviewCandidates: new-word query failed", "err", err, "user_id", userID, "level", level)
		return nil, fmt.Errorf("data.WordStore.ListReviewCandidates new-word query: %w", err)
	}
	fresh, err := scanReviewCandidatesSQLite(newRows, false)
	if closeErr := newRows.Close(); closeErr != nil {
		slog.Error("WordStore.ListReviewCandidates: close new-word rows failed", "err", closeErr)
	}
	if err != nil {
		slog.Error("WordStore.ListReviewCandidates: scan new-word rows failed", "err", err)
		return nil, err
	}

	candidates := append(due, fresh...)
	slog.Debug("WordStore.ListReviewCandidates done", "user_id", userID, "level", level,
		"due_count", len(due), "new_count", len(fresh))
	return candidates, nil
}

func scanReviewCandidatesSQLite(rows *sql.Rows, withRecord bool) ([]word.ReviewCandidate, error) {
	var result []word.ReviewCandidate
	for rows.Next() {
		var w word.Word
		var examplesJSON string
		dest := []any{
			&w.ID, &w.KanjiForm, &w.Reading, &w.PartOfSpeech, &w.Meaning,
			&examplesJSON, &w.JLPTLevel, &w.ReadingType, &w.AudioURL,
		}
		var r word.WordRecord
		var historyJSON, nextReviewAt, updatedAt string
		if withRecord {
			dest = append(dest,
				&r.ID, &r.UserID, &r.WordID, &r.MasteryLevel, &nextReviewAt,
				&r.EaseFactor, &r.Interval, &historyJSON, &updatedAt,
			)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("data.scanReviewCandidatesSQLite scan: %w", err)
		}

		if examplesJSON != "" {
			if err := json.Unmarshal([]byte(examplesJSON), &w.Examples); err != nil {
				return nil, fmt.Errorf("data.scanReviewCandidatesSQLite unmarshal examples for word %d: %w", w.ID, err)
			}
		}

		candidate := word.ReviewCandidate{Word: w}
		if withRecord {
			var err error
			r.NextReviewAt, err = parseSQLiteTime(nextReviewAt)
			if err != nil {
				return nil, fmt.Errorf("data.scanReviewCandidatesSQLite parse next_review_at: %w", err)
			}
			if updatedAt != "" {
				r.UpdatedAt, err = parseSQLiteTime(updatedAt)
				if err != nil {
					return nil, fmt.Errorf("data.scanReviewCandidatesSQLite parse updated_at: %w", err)
				}
			}
			if historyJSON != "" {
				if err := json.Unmarshal([]byte(historyJSON), &r.ReviewHistory); err != nil {
					return nil, fmt.Errorf("data.scanReviewCandidatesSQLite unmarshal history: %w", err)
				}
			}
			candidate.Record = &r
		}
		result = append(result, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data.scanReviewCandidatesSQLite rows: %w", err)
	}
	return result, nil
}
