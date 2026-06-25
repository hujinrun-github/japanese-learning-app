package testsuite

import (
	"context"
	"errors"
	"testing"
	"time"

	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/store"
)

type RuntimeFactory func(t *testing.T) *store.StoreRuntime

func RunTransactionContractTests(t *testing.T, newRuntime RuntimeFactory) {
	t.Helper()

	t.Run("commit", func(t *testing.T) {
		rt := newRuntime(t)
		email := uniqueTestEmail("commit")

		err := rt.Transact(context.Background(), func(tx *store.StoreRuntime) error {
			_, err := tx.App.Users.CreateUser(user.User{
				Name:       "Commit User",
				Email:      email,
				JLPTLevels: []string{string(user.LevelN5)},
			}, "hash")
			return err
		})
		if err != nil {
			t.Fatalf("Transact() error = %v", err)
		}

		got, _, err := rt.App.Users.GetUserByEmail(email)
		if err != nil {
			t.Fatalf("GetUserByEmail() error = %v", err)
		}
		if got == nil || got.Email != email {
			t.Fatalf("GetUserByEmail() email = %v, want %q", got, email)
		}
	})

	t.Run("rollback on error", func(t *testing.T) {
		rt := newRuntime(t)
		email := uniqueTestEmail("rollback")

		err := rt.Transact(context.Background(), func(tx *store.StoreRuntime) error {
			_, err := tx.App.Users.CreateUser(user.User{
				Name:       "Rollback User",
				Email:      email,
				JLPTLevels: []string{string(user.LevelN5)},
			}, "hash")
			if err != nil {
				return err
			}
			return errors.New("force rollback")
		})
		if err == nil {
			t.Fatal("Transact() error = nil, want force rollback")
		}

		_, _, err = rt.App.Users.GetUserByEmail(email)
		if !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("GetUserByEmail() error = %v, want ErrNotFound", err)
		}
	})

	t.Run("nested transaction", func(t *testing.T) {
		rt := newRuntime(t)

		err := rt.Transact(context.Background(), func(tx *store.StoreRuntime) error {
			return tx.Transact(context.Background(), func(*store.StoreRuntime) error {
				return nil
			})
		})
		if !errors.Is(err, store.ErrNestedTransaction) {
			t.Fatalf("Transact() error = %v, want ErrNestedTransaction", err)
		}
	})
}

func uniqueTestEmail(prefix string) string {
	return prefix + "-" + time.Now().UTC().Format("20060102150405.000000000") + "@example.com"
}
