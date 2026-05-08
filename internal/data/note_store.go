package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

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
