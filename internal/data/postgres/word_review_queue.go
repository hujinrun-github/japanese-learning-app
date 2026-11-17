package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"japanese-learning-app/internal/module/word"
)

// reviewWordColumns is the word projection shared by the two candidate
// queries. Examples are deliberately excluded and loaded afterwards for the
// selected ids only.
const reviewWordColumns = `w.id,
       w.kanji_form,
       w.reading,
       w.part_of_speech,
       w.meaning,
       w.jlpt_level,
       w.reading_type,
       w.verb_type,
       CASE WHEN ao.id IS NOT NULL THEN '/api/v1/audio/' || ao.id::text || '/stream' ELSE '' END`

const reviewAudioJoin = `LEFT JOIN audio_objects ao ON ao.id = w.audio_object_id AND ao.deleted_at IS NULL`

// ListReviewCandidates reads the two review groups in three lightweight
// queries: overdue words (capped at dueLimit), new words (capped at newLimit),
// and one batched example lookup for the selected ids. No whole-level word
// list and no per-word record lookup — both were the cause of multi-second
// queue requests on PostgreSQL.
func (s *WordStore) ListReviewCandidates(userID int64, level word.JLPTLevel, dueLimit, newLimit int) ([]word.ReviewCandidate, error) {
	slog.Debug("WordStore.ListReviewCandidates called", "user_id", userID, "level", level, "due_limit", dueLimit, "new_limit", newLimit)

	ctx := context.Background()
	now := s.now()
	localNow := now.In(s.location)
	todayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, s.location).UTC()
	if dueLimit < 1 {
		dueLimit = 1
	}
	if newLimit < 0 {
		newLimit = 0
	}

	dueRows, err := s.db.QueryContext(
		ctx,
		`SELECT `+reviewWordColumns+`,
		        wr.id, wr.user_id, wr.word_id, wr.mastery_level, wr.next_review_at,
		        wr.ease_factor, wr.interval_days, wr.review_history_json::text, wr.updated_at
		   FROM word_records wr
		   JOIN words w ON w.id = wr.word_id
		   `+reviewAudioJoin+`
		  WHERE wr.user_id = $1
		    AND w.jlpt_level = $2
		    AND wr.next_review_at <= $3
		    AND wr.updated_at < $4
		  ORDER BY wr.next_review_at ASC, w.id ASC
		  LIMIT $5`,
		userID, level, now.UTC(), todayStart, dueLimit,
	)
	if err != nil {
		slog.Error("WordStore.ListReviewCandidates: due query failed", "err", err, "user_id", userID, "level", level)
		return nil, fmt.Errorf("postgres.WordStore.ListReviewCandidates due query: %w", translateError(err))
	}
	due, err := scanReviewCandidates(dueRows, true)
	if closeErr := dueRows.Close(); closeErr != nil {
		slog.Error("WordStore.ListReviewCandidates: close due rows failed", "err", closeErr)
	}
	if err != nil {
		slog.Error("WordStore.ListReviewCandidates: scan due rows failed", "err", err)
		return nil, err
	}

	newRows, err := s.db.QueryContext(
		ctx,
		`SELECT `+reviewWordColumns+`
		   FROM words w
		   LEFT JOIN word_records wr ON wr.word_id = w.id AND wr.user_id = $1
		   `+reviewAudioJoin+`
		  WHERE w.jlpt_level = $2
		    AND wr.id IS NULL
		  ORDER BY w.id ASC
		  LIMIT $3`,
		userID, level, newLimit,
	)
	if err != nil {
		slog.Error("WordStore.ListReviewCandidates: new-word query failed", "err", err, "user_id", userID, "level", level)
		return nil, fmt.Errorf("postgres.WordStore.ListReviewCandidates new-word query: %w", translateError(err))
	}
	fresh, err := scanReviewCandidates(newRows, false)
	if closeErr := newRows.Close(); closeErr != nil {
		slog.Error("WordStore.ListReviewCandidates: close new-word rows failed", "err", closeErr)
	}
	if err != nil {
		slog.Error("WordStore.ListReviewCandidates: scan new-word rows failed", "err", err)
		return nil, err
	}

	candidates := append(due, fresh...)
	wordIDs := make([]int64, 0, len(candidates))
	for _, c := range candidates {
		wordIDs = append(wordIDs, c.Word.ID)
	}
	examples, err := s.loadExamplesByWordIDs(ctx, wordIDs)
	if err != nil {
		slog.Error("WordStore.ListReviewCandidates: load examples failed", "err", err)
		return nil, fmt.Errorf("postgres.WordStore.ListReviewCandidates load examples: %w", err)
	}
	for i := range candidates {
		candidates[i].Word.Examples = examples[candidates[i].Word.ID]
	}

	slog.Debug("WordStore.ListReviewCandidates done", "user_id", userID, "level", level,
		"due_count", len(due), "new_count", len(fresh), "examples_loaded", len(examples))
	return candidates, nil
}

// scanReviewCandidates scans either the overdue projection (word columns plus
// record columns) or the new-word projection (word columns only).
func scanReviewCandidates(rows *sql.Rows, withRecord bool) ([]word.ReviewCandidate, error) {
	var result []word.ReviewCandidate
	for rows.Next() {
		var w word.Word
		dest := []any{
			&w.ID, &w.KanjiForm, &w.Reading, &w.PartOfSpeech, &w.Meaning,
			&w.JLPTLevel, &w.ReadingType, &w.VerbType, &w.AudioURL,
		}
		var r word.WordRecord
		var historyJSON string
		if withRecord {
			dest = append(dest,
				&r.ID, &r.UserID, &r.WordID, &r.MasteryLevel, &r.NextReviewAt,
				&r.EaseFactor, &r.Interval, &historyJSON, &r.UpdatedAt,
			)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("postgres.scanReviewCandidates scan: %w", translateError(err))
		}

		candidate := word.ReviewCandidate{Word: w}
		if withRecord {
			if historyJSON != "" {
				if err := json.Unmarshal([]byte(historyJSON), &r.ReviewHistory); err != nil {
					return nil, fmt.Errorf("postgres.scanReviewCandidates unmarshal history: %w", err)
				}
			}
			candidate.Record = &r
		}
		result = append(result, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.scanReviewCandidates rows: %w", translateError(err))
	}
	return result, nil
}
