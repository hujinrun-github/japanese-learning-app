package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"japanese-learning-app/internal/module/shadowing"
)

type ShadowingStore struct {
	db queryer
}

func NewShadowingStore(db queryer) *ShadowingStore {
	return &ShadowingStore{db: db}
}

func (s *ShadowingStore) GetProgress(userID, lessonID int64, version int) (*shadowing.Progress, error) {
	row := s.db.QueryRowContext(
		context.Background(),
		`SELECT id, user_id, lesson_id, shadowing_version,
		        last_sentence_index, last_position_ms, last_practice_mode, updated_at
		 FROM lesson_shadowing_progress
		 WHERE user_id = $1 AND lesson_id = $2 AND shadowing_version = $3`,
		userID, lessonID, version,
	)

	var progress shadowing.Progress
	var mode string
	err := row.Scan(
		&progress.ID,
		&progress.UserID,
		&progress.LessonID,
		&progress.ShadowingVersion,
		&progress.LastSentenceIndex,
		&progress.LastPositionMS,
		&mode,
		&progress.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres.ShadowingStore.GetProgress scan: %w", translateError(err))
	}
	progress.LastPracticeMode = shadowing.PracticeMode(mode)

	return &progress, nil
}

func (s *ShadowingStore) UpsertProgress(p shadowing.Progress) error {
	_, err := s.db.ExecContext(
		context.Background(),
		`INSERT INTO lesson_shadowing_progress (
		     user_id, lesson_id, shadowing_version,
		     last_sentence_index, last_position_ms, last_practice_mode, updated_at
		 )
		 VALUES ($1, $2, $3, $4, $5, $6, now())
		 ON CONFLICT (user_id, lesson_id, shadowing_version) DO UPDATE SET
		     last_sentence_index = EXCLUDED.last_sentence_index,
		     last_position_ms = EXCLUDED.last_position_ms,
		     last_practice_mode = EXCLUDED.last_practice_mode,
		     updated_at = now()`,
		p.UserID,
		p.LessonID,
		p.ShadowingVersion,
		p.LastSentenceIndex,
		p.LastPositionMS,
		string(p.LastPracticeMode),
	)
	if err != nil {
		return fmt.Errorf("postgres.ShadowingStore.UpsertProgress exec: %w", translateError(err))
	}
	return nil
}

func (s *ShadowingStore) InsertAttempt(a shadowing.Attempt) error {
	var selfScore any
	if a.SelfScore != nil {
		selfScore = *a.SelfScore
	}

	_, err := s.db.ExecContext(
		context.Background(),
		`INSERT INTO lesson_shadowing_attempts (
		     user_id, lesson_id, shadowing_version, sentence_index,
		     practice_mode, playback_rate, loop_count, self_score,
		     recognition_text, audio_ref
		 )
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		a.UserID,
		a.LessonID,
		a.ShadowingVersion,
		a.SentenceIndex,
		string(a.PracticeMode),
		a.PlaybackRate,
		a.LoopCount,
		selfScore,
		a.RecognitionText,
		a.AudioRef,
	)
	if err != nil {
		return fmt.Errorf("postgres.ShadowingStore.InsertAttempt exec: %w", translateError(err))
	}
	return nil
}

func (s *ShadowingStore) ListAttemptSummary(userID, lessonID int64, version int) ([]shadowing.SentenceAttemptSummary, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT
		     a.sentence_index,
		     COUNT(*) AS attempt_count,
		     MAX(a.self_score) AS best_score,
		     (
		         SELECT latest.self_score
		         FROM lesson_shadowing_attempts latest
		         WHERE latest.user_id = a.user_id
		           AND latest.lesson_id = a.lesson_id
		           AND latest.shadowing_version = a.shadowing_version
		           AND latest.sentence_index = a.sentence_index
		           AND latest.self_score IS NOT NULL
		         ORDER BY latest.created_at DESC, latest.id DESC
		         LIMIT 1
		     ) AS last_score
		 FROM lesson_shadowing_attempts a
		 WHERE a.user_id = $1 AND a.lesson_id = $2 AND a.shadowing_version = $3
		 GROUP BY a.user_id, a.lesson_id, a.shadowing_version, a.sentence_index
		 ORDER BY a.sentence_index`,
		userID, lessonID, version,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.ShadowingStore.ListAttemptSummary query: %w", translateError(err))
	}
	defer rows.Close()

	var summary []shadowing.SentenceAttemptSummary
	for rows.Next() {
		var row shadowing.SentenceAttemptSummary
		var bestScore sql.NullInt64
		var lastScore sql.NullInt64
		if err := rows.Scan(&row.SentenceIndex, &row.AttemptCount, &bestScore, &lastScore); err != nil {
			return nil, fmt.Errorf("postgres.ShadowingStore.ListAttemptSummary scan: %w", translateError(err))
		}
		row.BestScore = nullableInt(bestScore)
		row.LastScore = nullableInt(lastScore)
		summary = append(summary, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.ShadowingStore.ListAttemptSummary rows: %w", translateError(err))
	}
	if summary == nil {
		summary = []shadowing.SentenceAttemptSummary{}
	}
	return summary, nil
}

func (s *ShadowingStore) ListCompletedSentenceIndexes(userID, lessonID int64, version int) ([]int, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT DISTINCT sentence_index
		 FROM lesson_shadowing_attempts
		 WHERE user_id = $1 AND lesson_id = $2 AND shadowing_version = $3
		 ORDER BY sentence_index`,
		userID, lessonID, version,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.ShadowingStore.ListCompletedSentenceIndexes query: %w", translateError(err))
	}
	defer rows.Close()

	var indexes []int
	for rows.Next() {
		var index int
		if err := rows.Scan(&index); err != nil {
			return nil, fmt.Errorf("postgres.ShadowingStore.ListCompletedSentenceIndexes scan: %w", translateError(err))
		}
		indexes = append(indexes, index)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.ShadowingStore.ListCompletedSentenceIndexes rows: %w", translateError(err))
	}
	if indexes == nil {
		indexes = []int{}
	}
	return indexes, nil
}

func nullableInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	intValue := int(value.Int64)
	return &intValue
}
