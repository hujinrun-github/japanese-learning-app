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
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	deleted := 0
	for _, group := range groups {
		for _, id := range group.DuplicateIDs {
			if id == group.KeptID {
				continue
			}
			if _, err = tx.Exec(`DELETE FROM lesson_shadowing_attempts WHERE lesson_id = ?`, id); err != nil {
				return 0, fmt.Errorf("cli.CleanupLessonDuplicates delete attempts for lesson %d: %w", id, err)
			}
			if _, err = tx.Exec(`DELETE FROM lesson_shadowing_progress WHERE lesson_id = ?`, id); err != nil {
				return 0, fmt.Errorf("cli.CleanupLessonDuplicates delete progress for lesson %d: %w", id, err)
			}
			result, execErr := tx.Exec(`DELETE FROM lessons WHERE id = ?`, id)
			if execErr != nil {
				err = execErr
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
