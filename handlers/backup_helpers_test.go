package handlers

// +parallel:serial — model.SetDriverForTesting
//
// TestBuildInsertStmt_* and TestBuildMultiRowInsert_* flip
// model.dbDriver between "sqlite" and "postgres" via
// model.SetDriverForTesting to exercise both placeholder branches.
// The other tests in this file are pure but live in the same file, so
// the whole file is serial.

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/model"
)

// ---------------------------------------------------------------------------
// normaliseValue
// ---------------------------------------------------------------------------

func TestNormaliseValue(t *testing.T) {
	t.Parallel()

	stamp := time.Date(2026, 4, 25, 12, 30, 0, 0, time.UTC)

	cases := []struct {
		name string
		in   interface{}
		want interface{}
	}{
		{"bytes → string", []byte("hello"), "hello"},
		{"empty bytes → empty string", []byte(nil), ""},
		{"time → RFC3339 string", stamp, stamp.Format(time.RFC3339)},
		{"int passthrough", int(42), int(42)},
		{"int64 passthrough", int64(7), int64(7)},
		{"float passthrough", 3.14, 3.14},
		{"string passthrough", "literal", "literal"},
		{"nil passthrough", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, normaliseValue(tc.in))
		})
	}
}

// ---------------------------------------------------------------------------
// columnsFromRow
// ---------------------------------------------------------------------------

func TestColumnsFromRow(t *testing.T) {
	t.Parallel()

	row := map[string]interface{}{
		"id":   1,
		"name": "abc",
		"flag": true,
	}
	got := columnsFromRow(row)
	sort.Strings(got)

	assert.Equal(t, []string{"flag", "id", "name"}, got)
}

// ---------------------------------------------------------------------------
// buildInsertStmt — branches on driver via model.IsPostgres()
// ---------------------------------------------------------------------------

func TestBuildInsertStmt_SQLite(t *testing.T) {
	prevDriver := model.GetDriver()
	model.SetDriverForTesting("sqlite")
	t.Cleanup(func() { model.SetDriverForTesting(prevDriver) })

	stmt := buildInsertStmt("zones", []string{"id", "name"})
	assert.Equal(t, `INSERT INTO zones (id, name) VALUES (?, ?)`, stmt)
}

func TestBuildInsertStmt_Postgres(t *testing.T) {
	prevDriver := model.GetDriver()
	model.SetDriverForTesting("postgres")
	t.Cleanup(func() { model.SetDriverForTesting(prevDriver) })

	stmt := buildInsertStmt("zones", []string{"id", "name"})
	assert.Equal(t, `INSERT INTO zones (id, name) VALUES ($1, $2)`, stmt)
}

// ---------------------------------------------------------------------------
// buildMultiRowInsert — must produce N×cols placeholders + flat args
// ---------------------------------------------------------------------------

func TestBuildMultiRowInsert_Postgres(t *testing.T) {
	prevDriver := model.GetDriver()
	model.SetDriverForTesting("postgres")
	t.Cleanup(func() { model.SetDriverForTesting(prevDriver) })

	rows := []map[string]interface{}{
		{"id": 1, "name": "a"},
		{"id": 2, "name": "b"},
	}
	stmt, args := buildMultiRowInsert("zones", []string{"id", "name"}, rows)

	// Two row-tuples, four args, sequential placeholder numbering.
	assert.True(t, strings.Contains(stmt, "VALUES ($1, $2), ($3, $4)"),
		"placeholder layout wrong: %q", stmt)
	require.Len(t, args, 4)
	assert.Equal(t, []interface{}{1, "a", 2, "b"}, args)
}

func TestBuildMultiRowInsert_SQLite(t *testing.T) {
	prevDriver := model.GetDriver()
	model.SetDriverForTesting("sqlite")
	t.Cleanup(func() { model.SetDriverForTesting(prevDriver) })

	rows := []map[string]interface{}{{"id": 1, "name": "a"}}
	stmt, _ := buildMultiRowInsert("zones", []string{"id", "name"}, rows)

	assert.True(t, strings.Contains(stmt, "VALUES (?, ?)"),
		"sqlite uses ? placeholders: %q", stmt)
}

// ---------------------------------------------------------------------------
// rowValues + coerceJSONValue
// ---------------------------------------------------------------------------

func TestRowValues_OrderMatchesCols(t *testing.T) {
	t.Parallel()

	row := map[string]interface{}{
		"a": 1,
		"b": 2,
		"c": 3,
	}
	got := rowValues([]string{"c", "a", "b"}, row)
	assert.Equal(t, []interface{}{3, 1, 2}, got)
}

func TestCoerceJSONValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   interface{}
		want interface{}
	}{
		{"int via json.Number", json.Number("42"), int64(42)},
		{"float via json.Number", json.Number("3.14"), 3.14},
		{"non-numeric json.Number falls back to string", json.Number("not-a-number"), "not-a-number"},
		{"plain int passthrough", 7, 7},
		{"plain string passthrough", "hi", "hi"},
		{"nil passthrough", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, coerceJSONValue(tc.in))
		})
	}
}
