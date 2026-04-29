package testutil

import (
	"io"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"isley/logger"
	"isley/model"
	"isley/utils"
)

// init prepares the singletons that production code expects to exist
// before any handler or DB query runs:
//
//   - logger.Log / logger.AccessWriter — silenced to io.Discard so
//     production code that logs unconditionally does not panic on nil
//     and does not pollute test output.
//   - utils.TranslationService — populated via utils.Init("en") so
//     handlers that call utils.TranslationService.GetTranslations(lang)
//     don't dereference a nil bundle.
//   - gin's debug writers — silenced so route-registration noise
//     (~100 lines per NewTestServer) does not drown real failures.
//   - model.dbDriver — set to "sqlite" so dialect-aware helpers
//     (IsSQLite/IsPostgres) work; production sets this inside InitDB,
//     which the harness sidesteps.
//
// We intentionally use package init() rather than a sync.Once-gated
// helper called from NewTestDB / NewTestServer. The earlier shape
// raced when a parallel test that did NOT go through NewTestDB (e.g.
// the grabber tests in watcher/grabber_parallel_test.go) read
// logger.Log from production code at the same time another test was
// triggering the lazy initialization. sync.Once synchronizes between
// callers of Do, not between Do's writes and an unrelated goroutine
// that never calls it. Package init() runs single-threaded before any
// test goroutine is launched and provides the happens-before
// relationship the lazy version was missing.
//
// We intentionally do not call logger.InitLogger(): that helper writes
// to logs/app.log via lumberjack, which would create real files in the
// repo's logs/ directory every test run. The test harness must have
// zero side effects on the working tree.
//
// Production code must never import this package.
func init() {
	l := logrus.New()
	l.SetOutput(io.Discard)
	l.SetLevel(logrus.PanicLevel)
	logger.Log = l
	logger.AccessWriter = io.Discard

	gin.SetMode(gin.TestMode)
	gin.DefaultWriter = io.Discard
	gin.DefaultErrorWriter = io.Discard

	model.SetDriverForTesting("sqlite")

	utils.Init("en")
}

// ensureProcessInitialized is retained as a no-op for backward
// compatibility with helpers that called it before the init()
// migration. New callers do not need to reach for it; importing
// testutil is sufficient.
func ensureProcessInitialized() {}
