package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"japanese-learning-app/internal/store"
)

type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type txBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

func newRuntime(db queryer, deps store.StoreDeps, transact func(context.Context, func(*store.StoreRuntime) error) error) *store.StoreRuntime {
	userStore := NewUserStore(db, deps)
	wordStore := NewWordStore(db, deps)
	grammarStore := NewGrammarStore(db, deps)
	lessonStore := NewLessonStore(db)
	speakingStore := NewSpeakingStore(db, deps)
	writingStore := NewWritingStore(db, deps)
	translationStore := NewTranslationStore(db, deps)

	return store.NewStoreRuntime(
		store.AppStores{
			Users:   userStore,
			Words:   wordStore,
			Grammar: grammarStore,
			Lessons: lessonStore,
		},
		store.AdminStores{
			Users:       userStore,
			Words:       wordStore,
			Grammar:     grammarStore,
			Speaking:    speakingStore,
			Writing:     writingStore,
			Translation: translationStore,
		},
		transact,
	)
}

func withQueryerTx(ctx context.Context, db queryer, fn func(queryer) error) error {
	switch typed := db.(type) {
	case *sql.Tx:
		return fn(typed)
	case txBeginner:
		tx, err := typed.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := fn(tx); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return errors.Join(err, rbErr)
			}
			return err
		}
		return tx.Commit()
	default:
		return fn(db)
	}
}

func currentTimeFunc(clock store.Clock) func() time.Time {
	if clock != nil {
		return clock.Now
	}
	return time.Now
}

func appLocation(loc *time.Location) *time.Location {
	if loc != nil {
		return loc
	}
	return time.UTC
}

func loggerOrNop(logger store.Logger) store.Logger {
	if logger != nil {
		return logger
	}
	return noopLogger{}
}

type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}
