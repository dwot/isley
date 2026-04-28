package app_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"isley/app"
	"isley/tests/testutil"
)

// TestNewEngine_HappyPath constructs an engine via the same path the
// integration tests use. The point of this test is per-package
// coverage reporting (Phase 8 of TEST_PLAN_2.md): every integration
// test already exercises NewEngine indirectly, but go test reports
// app/ at 0% because no _test.go file lives in the package. This file
// fixes the cosmetic.
func TestNewEngine_HappyPath(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	engine, err := app.NewEngine(app.Config{
		DB:            db,
		Assets:        testutil.RepoFS(t),
		Version:       "test",
		SessionSecret: []byte("isley-test-session-secret-32-by!"),
	})
	require.NoError(t, err)
	require.NotNil(t, engine)
}

// TestNewEngine_RejectsMissingDB verifies the constructor fails fast
// when the required DB handle is nil. Catches regressions where a
// new caller forgets to set Config.DB.
func TestNewEngine_RejectsMissingDB(t *testing.T) {
	t.Parallel()
	_, err := app.NewEngine(app.Config{})
	require.Error(t, err)
}
