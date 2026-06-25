package store

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrDuplicate         = errors.New("duplicate")
	ErrConstraint        = errors.New("constraint violation")
	ErrRetryable         = errors.New("retryable database error")
	ErrNestedTransaction = errors.New("nested transaction")
)
