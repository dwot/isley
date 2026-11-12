package service_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// forbidden lists calls the service layer must not make, with what to use
// instead. See the package documentation.
var forbidden = []struct {
	re  *regexp.Regexp
	use string
}{
	{regexp.MustCompile(`\btime\.Now\(`), "Deps.Clock"},
	{regexp.MustCompile(`\btime\.Local\b`), "Deps.Zones"},
	{regexp.MustCompile(`\.Local\(\)`), "Deps.Zones"},
	{regexp.MustCompile(`\butils\.AsLocal\b`), "Deps.Zones.FromWall"},
	{regexp.MustCompile(`\bmodel\.(IsPostgres|IsSQLite|GetDriver|GetDB)\b`), "Deps.Dialect / Deps.DB"},
	{regexp.MustCompile(`"github\.com/gin-gonic/gin"`), "nothing: services don't know about HTTP"},
}

func TestServiceConventions(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, err := os.ReadFile(file)
		require.NoError(t, err)
		for i, line := range strings.Split(string(src), "\n") {
			code := line
			if j := strings.Index(code, "//"); j >= 0 {
				code = code[:j]
			}
			for _, f := range forbidden {
				assert.False(t, f.re.MatchString(code), "%s:%d uses %s; use %s", file, i+1, f.re, f.use)
			}
		}
	}
}
