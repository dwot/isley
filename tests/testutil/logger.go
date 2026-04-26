package testutil

import (
	"io"
	"sync"

	"github.com/sirupsen/logrus"

	"isley/logger"
	"isley/utils"
)

// initOnce makes the harness idempotent: NewTestDB and NewTestServer can
// both call ensureProcessInitialized without racing each other.
var initOnce sync.Once

// ensureProcessInitialized prepares the singletons that production code
// expects to exist before any handler or DB query runs:
//
//   - logger.Log / logger.AccessWriter — silenced to io.Discard so
//     production code that logs unconditionally does not panic on nil
//     and does not pollute test output.
//   - utils.TranslationService — populated via utils.Init("en") so
//     handlers that call utils.TranslationService.GetTranslations(lang)
//     don't dereference a nil bundle.
//
// We intentionally do not call logger.InitLogger(): that helper writes
// to logs/app.log via lumberjack, which would create real files in the
// repo's logs/ directory every test run. The test harness must have
// zero side effects on the working tree.
func ensureProcessInitialized() {
	initOnce.Do(func() {
		l := logrus.New()
		l.SetOutput(io.Discard)
		l.SetLevel(logrus.PanicLevel)
		logger.Log = l
		logger.AccessWriter = io.Discard

		utils.Init("en")
	})
}
