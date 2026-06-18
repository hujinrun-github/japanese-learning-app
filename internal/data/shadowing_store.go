package data

import (
	"database/sql"
	"fmt"

	"japanese-learning-app/internal/module/shadowing"
)

type ShadowingStore struct {
	db *sql.DB
}

func NewShadowingStore(db *sql.DB) *ShadowingStore {
	return &ShadowingStore{db: db}
}

func (s *ShadowingStore) GetProgress(userID, lessonID int64, version int) (*shadowing.Progress, error) {
	row := s.db.QueryRow(`
		SELECT id, user_id, lesson_id, shadowing_version,
		       last_sentence_index, last_position_ms, last_practice_mode, updated_at
		FROM lesson_shadowing_progress
		WHERE user_id = ? AND lesson_id = ? AND shadowing_version = ?`,
		userID, lessonID, version,
	)

	var progress shadowing.Progress
	var mode string
	var updatedAt string
	err := row.Scan(
		&progress.ID,
		&progress.UserID,
		&progress.LessonID,
		&progress.ShadowingVersion,
		&progress.LastSentenceIndex,
		&progress.LastPositionMS,
		&mode,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("data.ShadowingStore.GetProgress scan: %w", err)
	}
	progress.LastPracticeMode = shadowing.PracticeMode(mode)
	progress.UpdatedAt, err = parseSQLiteTime(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("data.ShadowingStore.GetProgress parse updated_at: %w", err)
	}

	return &progress, nil
}

func (s *ShadowingStore) UpsertProgress(p shadowing.Progress) error {
	_, err := s.db.Exec(`
		INSERT INTO lesson_shadowing_progress (
			user_id, lesson_id, shadowing_version,
			last_sentence_index, last_position_ms, last_practice_mode, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(user_id, lesson_id, shadowing_version) DO UPDATE SET
			last_sentence_index = excluded.last_sentence_index,
			last_position_ms = excluded.last_position_ms,
			last_practice_mode = excluded.last_practice_mode,
			updated_at = datetime('now')`,
		p.UserID,
		p.LessonID,
		p.ShadowingVersion,
		p.LastSentenceIndex,
		p.LastPositionMS,
		string(p.LastPracticeMode),
	)
	if err != nil {
		return fmt.Errorf("data.ShadowingStore.UpsertProgress exec: %w", err)
	}
	return nil
}

func (s *ShadowingStore) InsertAttempt(a shadowing.Attempt) error {
	var selfScore any
	if a.SelfScore != nil {
		selfScore = *a.SelfScore
	}

	_, err := s.db.Exec(`
		INSERT INTO lesson_shadowing_attempts (
			user_id, lesson_id, shadowing_version, sentence_index,
			practice_mode, playback_rate, loop_count, self_score,
			recognition_text, audio_ref
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
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
		return fmt.Errorf("data.ShadowingStore.InsertAttempt exec: %w", err)
	}
	return nil
}

func (s *ShadowingStore) ListAttemptSummary(userID, lessonID int64, version int) ([]shadowing.SentenceAttemptSummary, error) {
	rows, err := s.db.Query(`
		SELECT
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
		WHERE a.user_id = ? AND a.lesson_id = ? AND a.shadowing_version = ?
		GROUP BY a.sentence_index
		ORDER BY a.sentence_index`,
		userID, lessonID, version,
	)
	if err != nil {
		return nil, fmt.Errorf("data.ShadowingStore.ListAttemptSummary query: %w", err)
	}
	defer rows.Close()

	var summary []shadowing.SentenceAttemptSummary
	for rows.Next() {
		var row shadowing.SentenceAttemptSummary
		var bestScore sql.NullInt64
		var lastScore sql.NullInt64
		if err := rows.Scan(&row.SentenceIndex, &row.AttemptCount, &bestScore, &lastScore); err != nil {
			return nil, fmt.Errorf("data.ShadowingStore.ListAttemptSummary scan: %w", err)
		}
		row.BestScore = nullableInt(bestScore)
		row.LastScore = nullableInt(lastScore)
		summary = append(summary, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data.ShadowingStore.ListAttemptSummary rows: %w", err)
	}
	if summary == nil {
		summary = []shadowing.SentenceAttemptSummary{}
	}
	return summary, nil
}

func (s *ShadowingStore) ListCompletedSentenceIndexes(userID, lessonID int64, version int) ([]int, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT sentence_index
		FROM lesson_shadowing_attempts
		WHERE user_id = ? AND lesson_id = ? AND shadowing_version = ?
		ORDER BY sentence_index`,
		userID, lessonID, version,
	)
	if err != nil {
		return nil, fmt.Errorf("data.ShadowingStore.ListCompletedSentenceIndexes query: %w", err)
	}
	defer rows.Close()

	var indexes []int
	for rows.Next() {
		var index int
		if err := rows.Scan(&index); err != nil {
			return nil, fmt.Errorf("data.ShadowingStore.ListCompletedSentenceIndexes scan: %w", err)
		}
		indexes = append(indexes, index)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data.ShadowingStore.ListCompletedSentenceIndexes rows: %w", err)
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
