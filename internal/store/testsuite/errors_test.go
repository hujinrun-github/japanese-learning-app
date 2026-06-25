package testsuite

import (
	"errors"
	"testing"

	"japanese-learning-app/internal/store"
)

func TestSentinelErrors(t *testing.T) {
	cases := []error{
		store.ErrNotFound,
		store.ErrDuplicate,
		store.ErrConstraint,
		store.ErrRetryable,
		store.ErrNestedTransaction,
	}

	for _, err := range cases {
		if !errors.Is(err, err) {
			t.Fatalf("sentinel error does not match itself: %v", err)
		}
	}
}
