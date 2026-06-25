package cli

import (
	"database/sql"
	"fmt"
)

type LessonDuplicateGroup struct {
	Title        string
	JLPTLevel    string
	DuplicateIDs []int64
	KeptID       int64
}

func ReportLessonDuplicates(db *sql.DB) ([]LessonDuplicateGroup, error) {
	rows, err := db.Query(`
		SELECT title, jlpt_level, MIN(id)
		FROM lessons
		GROUP BY title, jlpt_level
		HAVING COUNT(*) > 1
		ORDER BY title, jlpt_level
	`)
	if err != nil {
		return nil, fmt.Errorf("cli.ReportLessonDuplicates query groups: %w", err)
	}
	defer rows.Close()

	var groups []LessonDuplicateGroup
	for rows.Next() {
		var group LessonDuplicateGroup
		if err := rows.Scan(&group.Title, &group.JLPTLevel, &group.KeptID); err != nil {
			return nil, fmt.Errorf("cli.ReportLessonDuplicates scan group: %w", err)
		}

		ids, err := lessonIDsForTitleLevel(db, group.Title, group.JLPTLevel)
		if err != nil {
			return nil, err
		}
		group.DuplicateIDs = ids
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cli.ReportLessonDuplicates rows: %w", err)
	}

	return groups, nil
}

func CleanupLessonDuplicates(db *sql.DB) (int, error) {
	groups, err := ReportLessonDuplicates(db)
	if err != nil {
		return 0, err
	}
	if len(groups) == 0 {
		return 0, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("cli.CleanupLessonDuplicates Begin: %w", err)
	}
	defer tx.Rollback()

	deleted := 0
	for _, group := range groups {
		for _, id := range group.DuplicateIDs {
			if id == group.KeptID {
				continue
			}
			if err := moveShadowingAttempts(tx, id, group.KeptID); err != nil {
				return 0, err
			}
			if err := mergeShadowingProgress(tx, id, group.KeptID); err != nil {
				return 0, err
			}
			result, execErr := tx.Exec(`DELETE FROM lessons WHERE id = ?`, id)
			if execErr != nil {
				return 0, fmt.Errorf("cli.CleanupLessonDuplicates delete lesson %d: %w", id, execErr)
			}
			n, _ := result.RowsAffected()
			deleted += int(n)
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("cli.CleanupLessonDuplicates Commit: %w", err)
	}
	return deleted, nil
}

func moveShadowingAttempts(tx *sql.Tx, duplicateID, keptID int64) error {
	if _, err := tx.Exec(
		`UPDATE lesson_shadowing_attempts SET lesson_id = ? WHERE lesson_id = ?`,
		keptID,
		duplicateID,
	); err != nil {
		return fmt.Errorf("cli.moveShadowingAttempts %d -> %d: %w", duplicateID, keptID, err)
	}
	return nil
}

func mergeShadowingProgress(tx *sql.Tx, duplicateID, keptID int64) error {
	rows, err := tx.Query(`
		SELECT id,
		       user_id,
		       shadowing_version,
		       last_sentence_index,
		       last_position_ms,
		       last_practice_mode,
		       updated_at
		FROM lesson_shadowing_progress
		WHERE lesson_id = ?
		ORDER BY id`,
		duplicateID,
	)
	if err != nil {
		return fmt.Errorf("cli.mergeShadowingProgress query duplicate %d: %w", duplicateID, err)
	}
	defer rows.Close()

	type progressRow struct {
		id                int64
		userID            int64
		shadowingVersion  int
		lastSentenceIndex int
		lastPositionMS    int
		lastPracticeMode  string
		updatedAt         string
	}

	var duplicates []progressRow
	for rows.Next() {
		var row progressRow
		if err := rows.Scan(
			&row.id,
			&row.userID,
			&row.shadowingVersion,
			&row.lastSentenceIndex,
			&row.lastPositionMS,
			&row.lastPracticeMode,
			&row.updatedAt,
		); err != nil {
			return fmt.Errorf("cli.mergeShadowingProgress scan duplicate %d: %w", duplicateID, err)
		}
		duplicates = append(duplicates, row)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cli.mergeShadowingProgress rows duplicate %d: %w", duplicateID, err)
	}

	for _, duplicate := range duplicates {
		var keptProgressID int64
		var keptUpdatedAt string
		err := tx.QueryRow(`
			SELECT id, updated_at
			FROM lesson_shadowing_progress
			WHERE user_id = ? AND lesson_id = ? AND shadowing_version = ?`,
			duplicate.userID,
			keptID,
			duplicate.shadowingVersion,
		).Scan(&keptProgressID, &keptUpdatedAt)
		if err == sql.ErrNoRows {
			if _, err := tx.Exec(
				`UPDATE lesson_shadowing_progress SET lesson_id = ? WHERE id = ?`,
				keptID,
				duplicate.id,
			); err != nil {
				return fmt.Errorf("cli.mergeShadowingProgress move progress %d to lesson %d: %w", duplicate.id, keptID, err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("cli.mergeShadowingProgress find kept progress for duplicate %d: %w", duplicate.id, err)
		}

		if duplicate.updatedAt > keptUpdatedAt {
			if _, err := tx.Exec(`
				UPDATE lesson_shadowing_progress
				SET last_sentence_index = ?,
				    last_position_ms = ?,
				    last_practice_mode = ?,
				    updated_at = ?
				WHERE id = ?`,
				duplicate.lastSentenceIndex,
				duplicate.lastPositionMS,
				duplicate.lastPracticeMode,
				duplicate.updatedAt,
				keptProgressID,
			); err != nil {
				return fmt.Errorf("cli.mergeShadowingProgress update kept progress %d: %w", keptProgressID, err)
			}
		}
		if _, err := tx.Exec(`DELETE FROM lesson_shadowing_progress WHERE id = ?`, duplicate.id); err != nil {
			return fmt.Errorf("cli.mergeShadowingProgress delete duplicate progress %d: %w", duplicate.id, err)
		}
	}

	return nil
}

func CreateLessonUniqueIndex(db *sql.DB) error {
	groups, err := ReportLessonDuplicates(db)
	if err != nil {
		return err
	}
	if len(groups) > 0 {
		return fmt.Errorf("lesson duplicates exist; run cleanup-lesson-duplicates before creating unique index")
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_lessons_title_level_unique ON lessons (title, jlpt_level)`); err != nil {
		return fmt.Errorf("cli.CreateLessonUniqueIndex exec: %w", err)
	}
	return nil
}

func lessonIDsForTitleLevel(db *sql.DB, title, level string) ([]int64, error) {
	rows, err := db.Query(
		`SELECT id FROM lessons WHERE title = ? AND jlpt_level = ? ORDER BY id`,
		title,
		level,
	)
	if err != nil {
		return nil, fmt.Errorf("cli.lessonIDsForTitleLevel %q/%s: %w", title, level, err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cli.lessonIDsForTitleLevel scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cli.lessonIDsForTitleLevel rows: %w", err)
	}
	return ids, nil
}
