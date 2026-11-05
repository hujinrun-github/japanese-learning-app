package errata

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const (
	Pending  = "pending"
	Resolved = "resolved"
)

type Report struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	Module      string    `json:"module"`
	ItemKey     string    `json:"item_key"`
	ItemLabel   string    `json:"item_label"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Save(ctx context.Context, report Report) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO errata_reports
		(user_id, module, item_key, item_label, category, description)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		report.UserID, report.Module, report.ItemKey, report.ItemLabel, report.Category, report.Description,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("errata.Store.Save: %w", err)
	}
	return id, nil
}

func (s *Store) List(ctx context.Context, status string, limit, offset int) ([]Report, error) {
	return s.list(ctx, "($1 = '' OR status = $1)", status, limit, offset)
}

func (s *Store) ListByUser(ctx context.Context, userID int64, limit, offset int) ([]Report, error) {
	return s.list(ctx, "user_id = $1", userID, limit, offset)
}

func (s *Store) list(ctx context.Context, predicate string, scope any, limit, offset int) ([]Report, error) {
	// The predicate is fixed by the caller. User-supplied values are bound separately.
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, module, item_key, item_label, category, description, status, created_at
		FROM errata_reports WHERE `+predicate+`
		ORDER BY id DESC LIMIT $2 OFFSET $3`, scope, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("errata.Store.List query: %w", err)
	}
	defer rows.Close()

	reports := make([]Report, 0, limit)
	for rows.Next() {
		var report Report
		if err := rows.Scan(&report.ID, &report.UserID, &report.Module, &report.ItemKey, &report.ItemLabel,
			&report.Category, &report.Description, &report.Status, &report.CreatedAt); err != nil {
			return nil, fmt.Errorf("errata.Store.List scan: %w", err)
		}
		reports = append(reports, report)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("errata.Store.List rows: %w", err)
	}
	return reports, nil
}

func (s *Store) SetStatus(ctx context.Context, id int64, status string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE errata_reports SET status = $1 WHERE id = $2`, status, id)
	if err != nil {
		return false, fmt.Errorf("errata.Store.SetStatus update: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("errata.Store.SetStatus rows affected: %w", err)
	}
	return count > 0, nil
}
