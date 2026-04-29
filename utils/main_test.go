package utils

// TestMain runs before any test in the utils package. It silences the
// logger.Log / logger.AccessWriter package-globals once, synchronously,
// before any parallel test goroutine starts. Without this, the lazy
// silenceImageLogger() initializer in image_test.go (and the
// initI18nForTests path from i18n_test.go) would race against
// concurrent reads from production code paths under test —
// utils.GrabWebcamImage reads logger.Log on every call, and the
// validation and time helpers use it indirectly. The Phase 4 audit
// flipped every test in this package to t.Parallel(), which is what
// surfaced the race the lazy initializer had always papered over.
//
// Per docs/TEST_PLAN_2.md Phase 4: shared mutable state in tests is
// exactly what t.Parallel() is supposed to surface. This file is the
// fix; the lazy guards in silenceImageLogger and initI18nForTests stay
// idempotent so existing call sites do not need to change.

import (
	"io"
	"os"
	"testing"

	"github.com/sirupsen/logrus"

	"isley/logger"
)

func TestMain(m *testing.M) {
	// Silence the loggers up front, before any test goroutine starts.
	// Subsequent calls to silenceImageLogger from individual tests are
	// idempotent (the atomic.Bool short-circuits) so the existing call
	// sites are still correct; this just guarantees the writes happen
	// in a context with no concurrent readers.
	l := logrus.New()
	l.SetOutput(io.Discard)
	l.SetLevel(logrus.PanicLevel)
	logger.Log = l
	logger.AccessWriter = io.Discard
	imageLoggerOnce.Store(true)

	os.Exit(m.Run())
}
