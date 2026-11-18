package errata

import (
	"context"
	"database/sql"
	"errors"
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

// GetByIDForUser 读取指定用户自己的勘误。查不到时返回 found=false，
// 让调用方把「不存在」与「属于他人」统一按 404 处理，避免泄露他人记录。
func (s *Store) GetByIDForUser(ctx context.Context, id, userID int64) (Report, bool, error) {
	var report Report
	err := s.db.QueryRowContext(ctx, `SELECT id, user_id, module, item_key, item_label, category, description, status, created_at
		FROM errata_reports WHERE id = $1 AND user_id = $2`, id, userID).Scan(
		&report.ID, &report.UserID, &report.Module, &report.ItemKey, &report.ItemLabel,
		&report.Category, &report.Description, &report.Status, &report.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Report{}, false, nil
	}
	if err != nil {
		return Report{}, false, fmt.Errorf("errata.Store.GetByIDForUser: %w", err)
	}
	return report, true, nil
}

// UpdateForUser 只更新内容字段，并在 WHERE 中带上归属与 pending 状态，
// 防止并发情况下改到已被管理员处理或已被删除的报告。
func (s *Store) UpdateForUser(ctx context.Context, report Report) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE errata_reports
		SET item_label = $1, item_key = $2, category = $3, description = $4
		WHERE id = $5 AND user_id = $6 AND status = $7 AND (category <> 'audio' OR $3 = 'audio')`,
		report.ItemLabel, report.ItemKey, report.Category, report.Description,
		report.ID, report.UserID, Pending)
	if err != nil {
		return false, fmt.Errorf("errata.Store.UpdateForUser update: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("errata.Store.UpdateForUser rows affected: %w", err)
	}
	return count > 0, nil
}

// DeleteForUser 物理删除本人且仍待处理的勘误。
func (s *Store) DeleteForUser(ctx context.Context, id, userID int64) (bool, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM errata_reports WHERE id = $1 AND user_id = $2 AND status = $3 AND category <> 'audio'`,
		id, userID, Pending)
	if err != nil {
		return false, fmt.Errorf("errata.Store.DeleteForUser delete: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("errata.Store.DeleteForUser rows affected: %w", err)
	}
	return count > 0, nil
}
