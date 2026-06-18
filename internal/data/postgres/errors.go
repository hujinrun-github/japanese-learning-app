package postgres

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"japanese-learning-app/internal/store"
)

func (Adapter) TranslateError(err error) error {
	return translateError(err)
}

func translateError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return errors.Join(store.ErrNotFound, err)
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return errors.Join(store.ErrDuplicate, err)
		case "23503", "23514", "23502", "23P01":
			return errors.Join(store.ErrConstraint, err)
		case "40001", "40P01", "08000", "08003", "08006", "53300":
			return errors.Join(store.ErrRetryable, err)
		}
	}

	return err
}
