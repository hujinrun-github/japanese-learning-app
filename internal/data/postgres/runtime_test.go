package postgres

import (
	"context"
	"testing"

	"japanese-learning-app/internal/store"
	"japanese-learning-app/internal/store/testsuite"
)

func TestTransactionContracts(t *testing.T) {
	testsuite.RunTransactionContractTests(t, newPostgresRuntimeForTest)
}

func TestTransactBeginFailureReturnsError(t *testing.T) {
	db := openPostgresTestDB(t)
	_ = db.Close()

	rt, err := (Adapter{}).NewStoreRuntime(db, store.StoreDeps{})
	if err != nil {
		t.Fatalf("NewStoreRuntime() error = %v", err)
	}

	err = rt.Transact(context.Background(), func(*store.StoreRuntime) error {
		return nil
	})
	if err == nil {
		t.Fatal("Transact() error = nil, want begin error")
	}
}
