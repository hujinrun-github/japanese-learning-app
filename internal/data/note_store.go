package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"japanese-learning-app/internal/module/note"
)

// NoteStore implements persistence for notes, links, and FTS5 search.
type NoteStore struct {
	db *sql.DB
}

// NewNoteStore creates a NoteStore.
func NewNoteStore(db *sql.DB) *NoteStore {
	return &NoteStore{db: db}
}

// Create inserts a new note and sets its ID, CreatedAt, and UpdatedAt.
func (s *NoteStore) Create(n *note.Note) error {
	slog.Debug("NoteStore.Create called", "user_id", n.UserID, "type", n.Type)

	tagsJSON, err := json.Marshal(n.Tags)
	if err != nil {
		return fmt.Errorf("NoteStore.Create marshal tags: %w", err)
	}

	result, err := s.db.Exec(
		`INSERT INTO notes (user_id, type, title, content, source_text, reference_id, reference_type, tags_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		n.UserID, n.Type, n.Title, n.Content, n.SourceText, n.ReferenceID, n.ReferenceType, string(tagsJSON),
	)
	if err != nil {
		slog.Error("NoteStore.Create failed", "err", err)
		return fmt.Errorf("NoteStore.Create exec: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("NoteStore.Create LastInsertId: %w", err)
	}

	// Read back the created row to get server-generated timestamps
	created, err := s.GetByID(n.UserID, id)
	if err != nil {
		return fmt.Errorf("NoteStore.Create readback: %w", err)
	}
	*n = *created
	return nil
}

// GetByID returns a note by ID, filtering by user. Returns error if not found.
func (s *NoteStore) GetByID(userID, noteID int64) (*note.Note, error) {
	slog.Debug("NoteStore.GetByID called", "user_id", userID, "note_id", noteID)

	row := s.db.QueryRow(
		`SELECT id, user_id, type, title, content, source_text, reference_id, reference_type,
		        tags_json, mastery_level, next_review_at, ease_factor, interval,
		        review_history_json, created_at, updated_at
		 FROM notes WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		noteID, userID,
	)

	n, err := scanNote(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("NoteStore.GetByID note=%d user=%d: %w", noteID, userID, sql.ErrNoRows)
		}
		return nil, fmt.Errorf("NoteStore.GetByID: %w", err)
	}

	return n, nil
}

// List returns paginated notes for a user, with optional type/tag filtering and sorting.
func (s *NoteStore) List(userID int64, params note.NoteListParams) ([]note.Note, int, error) {
	slog.Debug("NoteStore.List called", "user_id", userID)

	sortCol := "updated_at"
	switch params.Sort {
	case "created_at":
		sortCol = "created_at"
	case "next_review_at":
		sortCol = "next_review_at"
	case "updated_at":
		sortCol = "updated_at"
	}
	order := "DESC"
	if params.Order == "asc" {
		order = "ASC"
	}

	where := "WHERE user_id = ? AND deleted_at IS NULL"
	args := []interface{}{userID}

	if params.Type != "" {
		where += " AND type = ?"
		args = append(args, string(params.Type))
	}
	if params.Tag != "" {
		where += " AND tags_json LIKE ?"
		args = append(args, fmt.Sprintf(`%%"%s"%%`, params.Tag))
	}

	// Count total
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM notes %s", where)
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		slog.Error("NoteStore.List count failed", "err", err)
		return nil, 0, fmt.Errorf("NoteStore.List count: %w", err)
	}

	// Query with sorting and pagination
	query := fmt.Sprintf(
		`SELECT id, user_id, type, title, content, source_text, reference_id, reference_type,
		        tags_json, mastery_level, next_review_at, ease_factor, interval,
		        review_history_json, created_at, updated_at
		 FROM notes %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		where, sortCol, order,
	)
	args = append(args, params.Limit, params.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		slog.Error("NoteStore.List query failed", "err", err)
		return nil, 0, fmt.Errorf("NoteStore.List query: %w", err)
	}
	defer rows.Close()

	var notes []note.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("NoteStore.List scan: %w", err)
		}
		notes = append(notes, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("NoteStore.List rows: %w", err)
	}

	slog.Debug("NoteStore.List done", "user_id", userID, "count", len(notes), "total", total)
	return notes, total, nil
}

// scanNote scans a single note from a row scanner.
func scanNote(scanner interface{ Scan(...interface{}) error }) (*note.Note, error) {
	var n note.Note
	var tagsJSON, historyJSON string
	var nextReviewAt sql.NullString
	var createdAt, updatedAt string
	var refID sql.NullInt64
	var refType sql.NullString

	err := scanner.Scan(
		&n.ID, &n.UserID, &n.Type, &n.Title, &n.Content, &n.SourceText,
		&refID, &refType, &tagsJSON, &n.MasteryLevel, &nextReviewAt, &n.EaseFactor,
		&n.Interval, &historyJSON, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if refID.Valid {
		n.ReferenceID = &refID.Int64
	}
	if refType.Valid {
		n.ReferenceType = &refType.String
	}

	if err := json.Unmarshal([]byte(tagsJSON), &n.Tags); err != nil {
		return nil, fmt.Errorf("scanNote unmarshal tags: %w", err)
	}

	if nextReviewAt.Valid {
		t, err := parseSQLiteTime(nextReviewAt.String)
		if err != nil {
			return nil, fmt.Errorf("scanNote parse next_review_at: %w", err)
		}
		n.NextReviewAt = &t
	}

	if err := json.Unmarshal([]byte(historyJSON), &n.ReviewHistory); err != nil {
		return nil, fmt.Errorf("scanNote unmarshal review_history: %w", err)
	}

	n.CreatedAt, err = parseSQLiteTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("scanNote parse created_at: %w", err)
	}
	n.UpdatedAt, err = parseSQLiteTime(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scanNote parse updated_at: %w", err)
	}

	return &n, nil
}

// Update updates all editable fields of a note. The updated_at timestamp is refreshed.
func (s *NoteStore) Update(n *note.Note) error {
	slog.Debug("NoteStore.Update called", "user_id", n.UserID, "note_id", n.ID)

	tagsJSON, err := json.Marshal(n.Tags)
	if err != nil {
		return fmt.Errorf("NoteStore.Update marshal tags: %w", err)
	}

	historyJSON, err := json.Marshal(n.ReviewHistory)
	if err != nil {
		return fmt.Errorf("NoteStore.Update marshal review_history: %w", err)
	}

	_, err = s.db.Exec(
		`UPDATE notes SET
		    type = ?, title = ?, content = ?, source_text = ?,
		    reference_id = ?, reference_type = ?, tags_json = ?,
		    mastery_level = ?, next_review_at = ?, ease_factor = ?,
		    interval = ?, review_history_json = ?, updated_at = datetime('now')
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		n.Type, n.Title, n.Content, n.SourceText,
		n.ReferenceID, n.ReferenceType, string(tagsJSON),
		n.MasteryLevel, formatSQLiteTimePtr(n.NextReviewAt), n.EaseFactor,
		n.Interval, string(historyJSON),
		n.ID, n.UserID,
	)
	if err != nil {
		slog.Error("NoteStore.Update failed", "err", err)
		return fmt.Errorf("NoteStore.Update exec: %w", err)
	}

	return nil
}

// SoftDelete marks a note as deleted by setting deleted_at.
func (s *NoteStore) SoftDelete(userID, noteID int64) error {
	slog.Debug("NoteStore.SoftDelete called", "user_id", userID, "note_id", noteID)

	result, err := s.db.Exec(
		`UPDATE notes SET deleted_at = datetime('now') WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		noteID, userID,
	)
	if err != nil {
		slog.Error("NoteStore.SoftDelete failed", "err", err)
		return fmt.Errorf("NoteStore.SoftDelete exec: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("NoteStore.SoftDelete: note %d not found or already deleted", noteID)
	}

	return nil
}

// formatSQLiteTimePtr formats a *time.Time for SQLite, returning nil if nil.
func formatSQLiteTimePtr(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return formatSQLiteTime(*t)
}
