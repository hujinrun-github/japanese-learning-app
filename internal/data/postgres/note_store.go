package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"japanese-learning-app/internal/module/note"
	"japanese-learning-app/internal/store"
)

// noteColumns is the shared SELECT list for notes. tags and review_history are
// returned as JSON text so they can be scanned without array driver support.
const noteColumns = `id, user_id, type, title, content, source_text, reference_id, reference_type,
	array_to_json(tags)::text, mastery_level, next_review_at, ease_factor, interval_days,
	review_history_json::text, created_at, updated_at`

// NoteStore implements note.NoteStoreInterface on top of PostgreSQL.
type NoteStore struct {
	db  queryer
	now func() time.Time
}

// NewNoteStore creates a NoteStore.
func NewNoteStore(db queryer, deps store.StoreDeps) *NoteStore {
	return &NoteStore{
		db:  db,
		now: currentTimeFunc(deps.Clock),
	}
}

func (s *NoteStore) Create(n *note.Note) error {
	tags := n.Tags
	if tags == nil {
		tags = []string{}
	}
	historyJSON, err := json.Marshal(orEmptySlice(n.ReviewHistory))
	if err != nil {
		return fmt.Errorf("postgres.NoteStore.Create marshal review_history: %w", err)
	}

	var id int64
	err = s.db.QueryRowContext(
		context.Background(),
		`INSERT INTO notes (user_id, type, title, content, source_text, reference_id, reference_type, tags, review_history_json)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::text[], $9::jsonb)
		 RETURNING id`,
		n.UserID,
		n.Type,
		n.Title,
		n.Content,
		n.SourceText,
		nullablePositiveInt64Ptr(n.ReferenceID),
		n.ReferenceType,
		tags,
		string(historyJSON),
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("postgres.NoteStore.Create: %w", translateError(err))
	}

	created, err := s.GetByID(n.UserID, id)
	if err != nil {
		return fmt.Errorf("postgres.NoteStore.Create readback: %w", err)
	}
	*n = *created
	return nil
}

func (s *NoteStore) GetByID(userID, noteID int64) (*note.Note, error) {
	row := s.db.QueryRowContext(
		context.Background(),
		`SELECT `+noteColumns+` FROM notes WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		noteID,
		userID,
	)
	n, err := scanNote(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("postgres.NoteStore.GetByID note=%d user=%d: %w", noteID, userID, sql.ErrNoRows)
		}
		return nil, err
	}
	if n == nil {
		return nil, fmt.Errorf("postgres.NoteStore.GetByID note=%d user=%d: %w", noteID, userID, sql.ErrNoRows)
	}
	return n, nil
}

func (s *NoteStore) List(userID int64, params note.NoteListParams) ([]note.Note, int, error) {
	sortCol := noteSortColumn(params.Sort)
	order := noteSortOrder(params.Order)

	where := `WHERE user_id = $1 AND deleted_at IS NULL`
	args := []any{userID}
	if params.Type != "" {
		args = append(args, string(params.Type))
		where += fmt.Sprintf(" AND type = $%d", len(args))
	}
	if params.Tag != "" {
		args = append(args, params.Tag)
		where += fmt.Sprintf(" AND $%d = ANY(tags)", len(args))
	}

	total, err := countRows(context.Background(), s.db, `SELECT COUNT(*) FROM notes `+where, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.NoteStore.List count: %w", err)
	}

	limit, offset := normalizePaging(params.Limit, params.Offset)
	args = append(args, limit, offset)
	query := fmt.Sprintf(
		`SELECT `+noteColumns+` FROM notes %s ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		where, sortCol, order, len(args)-1, len(args),
	)

	rows, err := s.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.NoteStore.List query: %w", translateError(err))
	}
	defer rows.Close()

	items, err := scanNotes(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *NoteStore) Update(n *note.Note) error {
	tags := n.Tags
	if tags == nil {
		tags = []string{}
	}
	historyJSON, err := json.Marshal(orEmptySlice(n.ReviewHistory))
	if err != nil {
		return fmt.Errorf("postgres.NoteStore.Update marshal review_history: %w", err)
	}

	_, err = s.db.ExecContext(
		context.Background(),
		`UPDATE notes SET
		    type = $3, title = $4, content = $5, source_text = $6,
		    reference_id = $7, reference_type = $8, tags = $9::text[],
		    mastery_level = $10, next_review_at = $11, ease_factor = $12,
		    interval_days = $13, review_history_json = $14::jsonb, updated_at = $15
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		n.ID,
		n.UserID,
		n.Type,
		n.Title,
		n.Content,
		n.SourceText,
		nullablePositiveInt64Ptr(n.ReferenceID),
		n.ReferenceType,
		tags,
		n.MasteryLevel,
		nullableTime(n.NextReviewAt),
		n.EaseFactor,
		n.Interval,
		string(historyJSON),
		s.now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres.NoteStore.Update: %w", translateError(err))
	}
	return nil
}

func (s *NoteStore) Search(userID int64, query string, limit int) ([]note.Note, error) {
	if limit <= 0 {
		limit = 20
	}
	pattern := "%" + query + "%"

	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT `+noteColumns+`
		 FROM notes
		 WHERE (title ILIKE $1 OR content ILIKE $1 OR source_text ILIKE $1)
		   AND user_id = $2 AND deleted_at IS NULL
		 ORDER BY updated_at DESC
		 LIMIT $3`,
		pattern,
		userID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.NoteStore.Search query: %w", translateError(err))
	}
	defer rows.Close()

	return scanNotes(rows)
}

func (s *NoteStore) SoftDelete(userID, noteID int64) error {
	result, err := s.db.ExecContext(
		context.Background(),
		`UPDATE notes SET deleted_at = $3 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		noteID,
		userID,
		s.now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres.NoteStore.SoftDelete: %w", translateError(err))
	}
	return requireRows(result, "postgres.NoteStore.SoftDelete", noteID)
}

// Restore clears a soft-delete marker after an external sync deletion was
// explicitly reversed by the user.
func (s *NoteStore) Restore(userID, noteID int64) error {
	result, err := s.db.ExecContext(
		context.Background(),
		`UPDATE notes SET deleted_at = NULL, updated_at = $3 WHERE id = $1 AND user_id = $2 AND deleted_at IS NOT NULL`,
		noteID,
		userID,
		s.now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres.NoteStore.Restore: %w", translateError(err))
	}
	return requireRows(result, "postgres.NoteStore.Restore", noteID)
}

func (s *NoteStore) AddLink(userID, noteID, targetNoteID int64, relation note.LinkRelation) (*note.NoteLink, error) {
	var id int64
	err := s.db.QueryRowContext(
		context.Background(),
		`INSERT INTO note_links (user_id, note_id, target_note_id, relation)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id`,
		userID,
		noteID,
		targetNoteID,
		string(relation),
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("postgres.NoteStore.AddLink: %w", translateError(err))
	}
	return &note.NoteLink{
		ID:           id,
		NoteID:       noteID,
		TargetNoteID: targetNoteID,
		Relation:     relation,
	}, nil
}

func (s *NoteStore) RemoveLink(userID, linkID int64) error {
	result, err := s.db.ExecContext(
		context.Background(),
		`DELETE FROM note_links WHERE id = $1 AND user_id = $2`,
		linkID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("postgres.NoteStore.RemoveLink: %w", translateError(err))
	}
	return requireRows(result, "postgres.NoteStore.RemoveLink", linkID)
}

func (s *NoteStore) GetOutgoingLinks(userID, noteID int64) ([]note.NoteLink, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT nl.id, nl.note_id, nl.target_note_id, nl.relation, n.id, n.title, n.type
		 FROM note_links nl
		 JOIN notes n ON nl.target_note_id = n.id
		 WHERE nl.user_id = $1 AND nl.note_id = $2 AND n.deleted_at IS NULL
		 ORDER BY nl.id ASC`,
		userID,
		noteID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.NoteStore.GetOutgoingLinks: %w", translateError(err))
	}
	defer rows.Close()

	return scanNoteLinks(rows)
}

func (s *NoteStore) GetIncomingLinks(userID, noteID int64) ([]note.NoteLink, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT nl.id, nl.note_id, nl.target_note_id, nl.relation, n.id, n.title, n.type
		 FROM note_links nl
		 JOIN notes n ON nl.note_id = n.id
		 WHERE nl.user_id = $1 AND nl.target_note_id = $2 AND n.deleted_at IS NULL
		 ORDER BY nl.id ASC`,
		userID,
		noteID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.NoteStore.GetIncomingLinks: %w", translateError(err))
	}
	defer rows.Close()

	return scanNoteLinks(rows)
}

func (s *NoteStore) Promote(userID, noteID int64) error {
	result, err := s.db.ExecContext(
		context.Background(),
		`UPDATE notes SET next_review_at = $3, updated_at = $3
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		noteID,
		userID,
		s.now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres.NoteStore.Promote: %w", translateError(err))
	}
	return requireRows(result, "postgres.NoteStore.Promote", noteID)
}

func (s *NoteStore) Demote(userID, noteID int64) error {
	result, err := s.db.ExecContext(
		context.Background(),
		`UPDATE notes SET next_review_at = NULL, updated_at = $3
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		noteID,
		userID,
		s.now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres.NoteStore.Demote: %w", translateError(err))
	}
	return requireRows(result, "postgres.NoteStore.Demote", noteID)
}

func (s *NoteStore) SaveReview(userID, noteID int64, n note.Note) error {
	historyJSON, err := json.Marshal(orEmptySlice(n.ReviewHistory))
	if err != nil {
		return fmt.Errorf("postgres.NoteStore.SaveReview marshal history: %w", err)
	}

	_, err = s.db.ExecContext(
		context.Background(),
		`UPDATE notes SET
		    mastery_level = $3, next_review_at = $4, ease_factor = $5, interval_days = $6,
		    review_history_json = $7::jsonb, updated_at = $8
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		noteID,
		userID,
		n.MasteryLevel,
		nullableTime(n.NextReviewAt),
		n.EaseFactor,
		n.Interval,
		string(historyJSON),
		s.now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres.NoteStore.SaveReview: %w", translateError(err))
	}
	return nil
}

func (s *NoteStore) ListDueNotes(userID int64) ([]note.Note, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT `+noteColumns+`
		 FROM notes
		 WHERE user_id = $1 AND next_review_at IS NOT NULL AND next_review_at <= $2
		       AND deleted_at IS NULL
		 ORDER BY next_review_at ASC`,
		userID,
		s.now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.NoteStore.ListDueNotes query: %w", translateError(err))
	}
	defer rows.Close()

	return scanNotes(rows)
}

func (s *NoteStore) ListArchived(userID int64, params note.NoteListParams) ([]note.Note, int, error) {
	sortCol := noteSortColumn(params.Sort)
	order := noteSortOrder(params.Order)

	where := `WHERE user_id = $1 AND mastery_level >= 5 AND next_review_at IS NULL AND deleted_at IS NULL`
	args := []any{userID}

	total, err := countRows(context.Background(), s.db, `SELECT COUNT(*) FROM notes `+where, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.NoteStore.ListArchived count: %w", err)
	}

	limit, offset := normalizePaging(params.Limit, params.Offset)
	args = append(args, limit, offset)
	query := fmt.Sprintf(
		`SELECT `+noteColumns+` FROM notes %s ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		where, sortCol, order, len(args)-1, len(args),
	)

	rows, err := s.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.NoteStore.ListArchived query: %w", translateError(err))
	}
	defer rows.Close()

	items, err := scanNotes(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *NoteStore) ListByReference(userID int64, refType string, refID int64, limit int) ([]note.NoteDigest, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT id, title, type FROM notes
		 WHERE user_id = $1 AND reference_type = $2 AND reference_id = $3 AND deleted_at IS NULL
		 ORDER BY updated_at DESC LIMIT $4`,
		userID,
		refType,
		refID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.NoteStore.ListByReference: %w", translateError(err))
	}
	defer rows.Close()

	digests := make([]note.NoteDigest, 0)
	for rows.Next() {
		var d note.NoteDigest
		if err := rows.Scan(&d.ID, &d.Title, &d.Type); err != nil {
			return nil, fmt.Errorf("postgres.NoteStore.ListByReference scan: %w", translateError(err))
		}
		digests = append(digests, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.NoteStore.ListByReference rows: %w", translateError(err))
	}
	return digests, nil
}

func (s *NoteStore) ListTags(userID int64) ([]string, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT DISTINCT unnest(tags) AS tag FROM notes
		 WHERE user_id = $1 AND deleted_at IS NULL
		 ORDER BY tag ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres.NoteStore.ListTags: %w", translateError(err))
	}
	defer rows.Close()

	tags := make([]string, 0)
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("postgres.NoteStore.ListTags scan: %w", translateError(err))
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.NoteStore.ListTags rows: %w", translateError(err))
	}
	return tags, nil
}

// ── helpers ────────────────────────────────────────────────────────────────

func scanNotes(rows *sql.Rows) ([]note.Note, error) {
	items := make([]note.Note, 0)
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		if n == nil {
			continue
		}
		items = append(items, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.scanNotes rows: %w", translateError(err))
	}
	return items, nil
}

type noteScanner interface {
	Scan(dest ...any) error
}

func scanNote(row noteScanner) (*note.Note, error) {
	var n note.Note
	var tagsJSON, historyJSON string
	var nextReviewAt sql.NullTime
	var refID sql.NullInt64
	var refType sql.NullString

	if err := row.Scan(
		&n.ID, &n.UserID, &n.Type, &n.Title, &n.Content, &n.SourceText,
		&refID, &refType, &tagsJSON, &n.MasteryLevel, &nextReviewAt, &n.EaseFactor,
		&n.Interval, &historyJSON, &n.CreatedAt, &n.UpdatedAt,
	); err != nil {
		if errors.Is(translateError(err), sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("postgres.scanNote scan: %w", translateError(err))
	}

	if refID.Valid {
		value := refID.Int64
		n.ReferenceID = &value
	}
	if refType.Valid {
		value := refType.String
		n.ReferenceType = &value
	}
	if nextReviewAt.Valid {
		value := nextReviewAt.Time
		n.NextReviewAt = &value
	}

	if tagsJSON != "" {
		if err := json.Unmarshal([]byte(tagsJSON), &n.Tags); err != nil {
			return nil, fmt.Errorf("postgres.scanNote unmarshal tags: %w", err)
		}
	}
	if n.Tags == nil {
		n.Tags = []string{}
	}
	if historyJSON != "" {
		if err := json.Unmarshal([]byte(historyJSON), &n.ReviewHistory); err != nil {
			return nil, fmt.Errorf("postgres.scanNote unmarshal review_history: %w", err)
		}
	}

	return &n, nil
}

func scanNoteLinks(rows *sql.Rows) ([]note.NoteLink, error) {
	links := make([]note.NoteLink, 0)
	for rows.Next() {
		var l note.NoteLink
		var digest note.NoteDigest
		if err := rows.Scan(
			&l.ID, &l.NoteID, &l.TargetNoteID, &l.Relation,
			&digest.ID, &digest.Title, &digest.Type,
		); err != nil {
			return nil, fmt.Errorf("postgres.scanNoteLinks scan: %w", translateError(err))
		}
		l.TargetNote = &digest
		links = append(links, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.scanNoteLinks rows: %w", translateError(err))
	}
	return links, nil
}

func noteSortColumn(sort string) string {
	switch sort {
	case "created_at":
		return "created_at"
	case "next_review_at":
		return "next_review_at"
	default:
		return "updated_at"
	}
}

func noteSortOrder(order string) string {
	if strings.EqualFold(order, "asc") {
		return "ASC"
	}
	return "DESC"
}

func normalizePaging(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func nullablePositiveInt64Ptr(value *int64) any {
	if value == nil || *value <= 0 {
		return nil
	}
	return *value
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func orEmptySlice(value any) any {
	if value == nil {
		return []string{}
	}
	return value
}

func requireRows(result sql.Result, op string, id int64) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s rows affected: %w", op, translateError(err))
	}
	if affected == 0 {
		return fmt.Errorf("%s: id %d: %w", op, id, sql.ErrNoRows)
	}
	return nil
}
