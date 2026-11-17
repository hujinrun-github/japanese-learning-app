// Package mistake stores the questions a learner answered incorrectly so they
// can be reviewed and marked as mastered later.
package mistake

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

const (
	Unmastered = "unmastered"
	Mastered   = "mastered"
)

// Mistake is one collected wrong answer.
type Mistake struct {
	ID            int64     `json:"id"`
	UserID        int64     `json:"user_id"`
	Module        string    `json:"module"`
	ItemKey       string    `json:"item_key"`
	Prompt        string    `json:"prompt"`
	UserAnswer    string    `json:"user_answer"`
	CorrectAnswer string    `json:"correct_answer"`
	Explanation   string    `json:"explanation"`
	SourcePath    string    `json:"source_path"`
	Status        string    `json:"status"`
	WrongCount    int       `json:"wrong_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Input is the payload accepted when a learner answers a question wrong.
type Input struct {
	Module        string `json:"module"`
	ItemKey       string `json:"item_key"`
	Prompt        string `json:"prompt"`
	UserAnswer    string `json:"user_answer"`
	CorrectAnswer string `json:"correct_answer"`
	Explanation   string `json:"explanation"`
	SourcePath    string `json:"source_path"`
}

// Page is the paginated list response.
type Page struct {
	Items []Mistake `json:"items"`
	Total int       `json:"total"`
	Page  int       `json:"page"`
	Size  int       `json:"size"`
}

// Store persists mistakes in the shared SQL database. The SQL uses numbered
// placeholders so the same code works with SQLite and PostgreSQL.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Save inserts a mistake, or refreshes an existing one when the same question
// is answered wrong again. A repeated mistake reopens a mastered row.
func (s *Store) Save(ctx context.Context, userID int64, in Input) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO mistakes
		(user_id, module, item_key, prompt, user_answer, correct_answer, explanation, source_path)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT(user_id, module, item_key) DO UPDATE SET
			prompt = excluded.prompt,
			user_answer = excluded.user_answer,
			correct_answer = excluded.correct_answer,
			explanation = excluded.explanation,
			source_path = excluded.source_path,
			status = 'unmastered',
			mastered_at = NULL,
			wrong_count = mistakes.wrong_count + 1,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id`,
		userID, in.Module, in.ItemKey, in.Prompt, in.UserAnswer, in.CorrectAnswer, in.Explanation, in.SourcePath,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("mistake.Store.Save: %w", err)
	}
	slog.Debug("mistake saved", "user_id", userID, "module", in.Module, "item_key", in.ItemKey, "id", id)
	return id, nil
}

// List returns mistakes for one user. Empty module/status means "no filter";
// the caller normalizes user-supplied query values before calling.
func (s *Store) List(ctx context.Context, userID int64, module, status string, limit, offset int) ([]Mistake, int, error) {
	const predicate = `user_id = $1 AND ($2 = '' OR module = $2) AND ($3 = '' OR status = $3)`

	var total int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mistakes WHERE `+predicate, userID, module, status).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("mistake.Store.List count: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, module, item_key, prompt, user_answer,
		correct_answer, explanation, source_path, status, wrong_count, created_at, updated_at
		FROM mistakes WHERE `+predicate+` ORDER BY updated_at DESC, id DESC LIMIT $4 OFFSET $5`,
		userID, module, status, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("mistake.Store.List query: %w", err)
	}
	defer rows.Close()

	items := make([]Mistake, 0, limit)
	for rows.Next() {
		var item Mistake
		if err := rows.Scan(&item.ID, &item.UserID, &item.Module, &item.ItemKey, &item.Prompt,
			&item.UserAnswer, &item.CorrectAnswer, &item.Explanation, &item.SourcePath,
			&item.Status, &item.WrongCount, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("mistake.Store.List scan: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("mistake.Store.List rows: %w", err)
	}
	slog.Debug("mistakes listed", "user_id", userID, "module", module, "status", status, "total", total, "returned", len(items))
	return items, total, nil
}

// SetStatus marks a mistake as mastered or reopens it. It is scoped to the
// owner so one learner cannot modify another learner's mistakes.
func (s *Store) SetStatus(ctx context.Context, userID, id int64, status string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE mistakes SET
		status = $1,
		mastered_at = CASE WHEN $1 = 'mastered' THEN CURRENT_TIMESTAMP ELSE NULL END,
		updated_at = CURRENT_TIMESTAMP
		WHERE id = $2 AND user_id = $3`, status, id, userID)
	if err != nil {
		return false, fmt.Errorf("mistake.Store.SetStatus update: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("mistake.Store.SetStatus rows affected: %w", err)
	}
	slog.Debug("mistake status updated", "user_id", userID, "id", id, "status", status, "updated", count > 0)
	return count > 0, nil
}
