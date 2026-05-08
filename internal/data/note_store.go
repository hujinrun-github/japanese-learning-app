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

// Search performs FTS5 full-text search on title, content, and source_text.
// Results are joined with notes table to filter by user and soft-delete.
func (s *NoteStore) Search(userID int64, query string, limit int) ([]note.Note, error) {
	slog.Debug("NoteStore.Search called", "user_id", userID, "query", query)

	likePattern := "%" + query + "%"
	rows, err := s.db.Query(
		`SELECT id, user_id, type, title, content, source_text,
		        reference_id, reference_type, tags_json,
		        mastery_level, next_review_at, ease_factor, interval,
		        review_history_json, created_at, updated_at
		 FROM notes
		 WHERE (title LIKE ? OR content LIKE ? OR source_text LIKE ?)
		   AND user_id = ? AND deleted_at IS NULL
		 ORDER BY updated_at DESC
		 LIMIT ?`,
		likePattern, likePattern, likePattern, userID, limit,
	)
	if err != nil {
		slog.Error("NoteStore.Search query failed", "err", err)
		return nil, fmt.Errorf("NoteStore.Search: %w", err)
	}
	defer rows.Close()

	var notes []note.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, fmt.Errorf("NoteStore.Search scan: %w", err)
		}
		notes = append(notes, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("NoteStore.Search rows: %w", err)
	}

	slog.Debug("NoteStore.Search done", "user_id", userID, "count", len(notes))
	return notes, nil
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

// AddLink creates a link between two notes.
func (s *NoteStore) AddLink(userID, noteID, targetNoteID int64, relation note.LinkRelation) (*note.NoteLink, error) {
	slog.Debug("NoteStore.AddLink called", "user_id", userID)

	result, err := s.db.Exec(
		`INSERT INTO note_links (user_id, note_id, target_note_id, relation)
		 VALUES (?, ?, ?, ?)`,
		userID, noteID, targetNoteID, string(relation),
	)
	if err != nil {
		slog.Error("NoteStore.AddLink failed", "err", err)
		return nil, fmt.Errorf("NoteStore.AddLink: %w", err)
	}

	id, _ := result.LastInsertId()
	return &note.NoteLink{
		ID:           id,
		NoteID:       noteID,
		TargetNoteID: targetNoteID,
		Relation:     relation,
	}, nil
}

// RemoveLink deletes a link by ID.
func (s *NoteStore) RemoveLink(userID, linkID int64) error {
	slog.Debug("NoteStore.RemoveLink called", "user_id", userID, "link_id", linkID)

	result, err := s.db.Exec(
		`DELETE FROM note_links WHERE id = ? AND user_id = ?`,
		linkID, userID,
	)
	if err != nil {
		slog.Error("NoteStore.RemoveLink failed", "err", err)
		return fmt.Errorf("NoteStore.RemoveLink: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("NoteStore.RemoveLink: link %d not found", linkID)
	}
	return nil
}

// GetOutgoingLinks returns all links from a note to others, with target note digests populated.
func (s *NoteStore) GetOutgoingLinks(userID, noteID int64) ([]note.NoteLink, error) {
	slog.Debug("NoteStore.GetOutgoingLinks called", "user_id", userID, "note_id", noteID)

	rows, err := s.db.Query(
		`SELECT nl.id, nl.note_id, nl.target_note_id, nl.relation,
		        n.id, n.title, n.type
		 FROM note_links nl
		 JOIN notes n ON nl.target_note_id = n.id
		 WHERE nl.user_id = ? AND nl.note_id = ? AND n.deleted_at IS NULL`,
		userID, noteID,
	)
	if err != nil {
		return nil, fmt.Errorf("NoteStore.GetOutgoingLinks: %w", err)
	}
	defer rows.Close()

	return scanNoteLinks(rows)
}

// GetIncomingLinks returns all links from other notes to this note (backlinks).
func (s *NoteStore) GetIncomingLinks(userID, noteID int64) ([]note.NoteLink, error) {
	slog.Debug("NoteStore.GetIncomingLinks called", "user_id", userID, "note_id", noteID)

	rows, err := s.db.Query(
		`SELECT nl.id, nl.note_id, nl.target_note_id, nl.relation,
		        n.id, n.title, n.type
		 FROM note_links nl
		 JOIN notes n ON nl.note_id = n.id
		 WHERE nl.user_id = ? AND nl.target_note_id = ? AND n.deleted_at IS NULL`,
		userID, noteID,
	)
	if err != nil {
		return nil, fmt.Errorf("NoteStore.GetIncomingLinks: %w", err)
	}
	defer rows.Close()

	return scanNoteLinks(rows)
}

func scanNoteLinks(rows *sql.Rows) ([]note.NoteLink, error) {
	var links []note.NoteLink
	for rows.Next() {
		var l note.NoteLink
		var digest note.NoteDigest
		if err := rows.Scan(&l.ID, &l.NoteID, &l.TargetNoteID, &l.Relation,
			&digest.ID, &digest.Title, &digest.Type); err != nil {
			return nil, fmt.Errorf("scanNoteLinks: %w", err)
		}
		l.TargetNote = &digest
		links = append(links, l)
	}
	return links, rows.Err()
}

// formatSQLiteTimePtr formats a *time.Time for SQLite, returning nil if nil.
func formatSQLiteTimePtr(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return formatSQLiteTime(*t)
}

// Promote sets next_review_at to now, adding the note to the review queue.
func (s *NoteStore) Promote(userID, noteID int64) error {
	slog.Debug("NoteStore.Promote called", "user_id", userID, "note_id", noteID)

	result, err := s.db.Exec(
		`UPDATE notes SET next_review_at = datetime('now'), updated_at = datetime('now')
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		noteID, userID,
	)
	if err != nil {
		slog.Error("NoteStore.Promote failed", "err", err)
		return fmt.Errorf("NoteStore.Promote: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("NoteStore.Promote: note %d not found", noteID)
	}
	return nil
}

// Demote removes the note from the review queue by setting next_review_at to NULL.
func (s *NoteStore) Demote(userID, noteID int64) error {
	slog.Debug("NoteStore.Demote called", "user_id", userID, "note_id", noteID)

	result, err := s.db.Exec(
		`UPDATE notes SET next_review_at = NULL, updated_at = datetime('now')
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		noteID, userID,
	)
	if err != nil {
		slog.Error("NoteStore.Demote failed", "err", err)
		return fmt.Errorf("NoteStore.Demote: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("NoteStore.Demote: note %d not found", noteID)
	}
	return nil
}

// SaveReview persists SM-2 review results (mastery, interval, EF, next_review_at, history).
func (s *NoteStore) SaveReview(userID, noteID int64, n note.Note) error {
	slog.Debug("NoteStore.SaveReview called", "user_id", userID, "note_id", noteID)

	historyJSON, err := json.Marshal(n.ReviewHistory)
	if err != nil {
		return fmt.Errorf("NoteStore.SaveReview marshal history: %w", err)
	}

	_, err = s.db.Exec(
		`UPDATE notes SET
		    mastery_level = ?, next_review_at = ?, ease_factor = ?, interval = ?,
		    review_history_json = ?, updated_at = datetime('now')
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		n.MasteryLevel, formatSQLiteTimePtr(n.NextReviewAt), n.EaseFactor,
		n.Interval, string(historyJSON),
		noteID, userID,
	)
	if err != nil {
		slog.Error("NoteStore.SaveReview failed", "err", err)
		return fmt.Errorf("NoteStore.SaveReview: %w", err)
	}
	return nil
}

// ListDueNotes returns notes due for review (next_review_at <= now, not deleted).
func (s *NoteStore) ListDueNotes(userID int64) ([]note.Note, error) {
	slog.Debug("NoteStore.ListDueNotes called", "user_id", userID)

	rows, err := s.db.Query(
		`SELECT id, user_id, type, title, content, source_text, reference_id, reference_type,
		        tags_json, mastery_level, next_review_at, ease_factor, interval,
		        review_history_json, created_at, updated_at
		 FROM notes
		 WHERE user_id = ? AND next_review_at IS NOT NULL AND next_review_at <= datetime('now')
		       AND deleted_at IS NULL
		 ORDER BY next_review_at ASC`,
		userID,
	)
	if err != nil {
		slog.Error("NoteStore.ListDueNotes query failed", "err", err)
		return nil, fmt.Errorf("NoteStore.ListDueNotes: %w", err)
	}
	defer rows.Close()

	var notes []note.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, fmt.Errorf("NoteStore.ListDueNotes scan: %w", err)
		}
		notes = append(notes, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("NoteStore.ListDueNotes rows: %w", err)
	}

	slog.Debug("NoteStore.ListDueNotes done", "user_id", userID, "count", len(notes))
	return notes, nil
}

// ListArchived returns graduated notes (mastery >= 5, next_review_at IS NULL, not deleted).
func (s *NoteStore) ListArchived(userID int64, params note.NoteListParams) ([]note.Note, int, error) {
	slog.Debug("NoteStore.ListArchived called", "user_id", userID)

	where := "WHERE user_id = ? AND mastery_level >= 5 AND next_review_at IS NULL AND deleted_at IS NULL"
	args := []interface{}{userID}

	sortCol := "updated_at"
	if params.Sort == "created_at" {
		sortCol = "created_at"
	}
	order := "DESC"
	if params.Order == "asc" {
		order = "ASC"
	}

	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM notes %s", where)
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		slog.Error("NoteStore.ListArchived count failed", "err", err)
		return nil, 0, fmt.Errorf("NoteStore.ListArchived count: %w", err)
	}

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
		slog.Error("NoteStore.ListArchived query failed", "err", err)
		return nil, 0, fmt.Errorf("NoteStore.ListArchived query: %w", err)
	}
	defer rows.Close()

	var notes []note.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("NoteStore.ListArchived scan: %w", err)
		}
		notes = append(notes, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("NoteStore.ListArchived rows: %w", err)
	}

	slog.Debug("NoteStore.ListArchived done", "user_id", userID, "count", len(notes), "total", total)
	return notes, total, nil
}
