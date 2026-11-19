package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
)

func (h *Handler) resolveErrataContent(ctx context.Context, id int64, revision string) error {
	if h.cfg.DB == nil {
		return contentProblem(503, "内容数据库尚未配置")
	}
	tx, err := h.cfg.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin errata resolution: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			slog.ErrorContext(ctx, "rollback errata resolution failed", "err", err)
		}
	}()
	doc, err := loadErrataContentDocument(ctx, tx, id, true)
	if err != nil {
		return err
	}
	if doc.content.Revision != revision {
		return contentProblem(409, "原内容已变化，请重新核查后再标记已处理")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE errata_reports SET status='resolved' WHERE id=$1`, id); err != nil {
		return fmt.Errorf("resolve errata after verification: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit errata resolution: %w", err)
	}
	return nil
}
