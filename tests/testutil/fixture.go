package testutil

// Fixture loaders for tests/fixtures/*. Phase 2 of docs/TEST_PLAN.md
// pulls inline canned bodies (`aciCannedJSON`, `ecowittCannedJSON`,
// hand-built valid backup zips, etc.) into versioned files under
// tests/fixtures/ so the same payload can be reused across test files.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// MustReadFixture loads a file from tests/fixtures/. The path is
// relative to the fixtures root. Test failure on miss is the right
// behavior — a missing fixture is a test-author bug, not a runtime
// condition.
func MustReadFixture(t *testing.T, path string) []byte {
	t.Helper()
	root, err := repoRoot()
	require.NoError(t, err, "MustReadFixture: locate repo root")
	full := filepath.Join(root, "tests", "fixtures", path)
	data, err := os.ReadFile(full)
	require.NoErrorf(t, err, "MustReadFixture: read %s", full)
	return data
}

// MustLoadJSONFixture is a typed convenience wrapper for the
// JSON-shaped fixtures. T is the target struct or map type.
func MustLoadJSONFixture[T any](t *testing.T, path string) T {
	t.Helper()
	var out T
	require.NoError(t, json.Unmarshal(MustReadFixture(t, path), &out),
		"MustLoadJSONFixture: decode %s", path)
	return out
}
