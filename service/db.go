package service

import (
	"context"
	"database/sql"
	"fmt"
)

// querier is what *sql.DB and *sql.Tx have in common, so a read can run
// inside or outside a transaction.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// inTx runs fn in a transaction and commits if it returns nil.
func inTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// nullFloat returns a pointer to f's value, or nil when it is NULL.
func nullFloat(f sql.NullFloat64) *float64 {
	if !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}

// nullInt returns a pointer to n's value, or nil when it is NULL.
func nullInt(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// Patch is one field of a partial update. Set says the caller sent the
// field; Value is its new value, and nil clears it.
type Patch[T any] struct {
	Set   bool
	Value *T
}

// Put returns a Patch that sets the field to v (nil clears it).
func Put[T any](v *T) Patch[T] { return Patch[T]{Set: true, Value: v} }
