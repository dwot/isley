package chrono_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// boundaryRules are the ways code used to convert times outside this
// package. Stored times are turned into instants (and back) only by
// chrono.Zones; the server's local zone is never a stand-in for the
// instance zone.
var boundaryRules = []struct {
	re  *regexp.Regexp
	why string
}{
	{regexp.MustCompile(`\.Local\(\)`), "use chrono.Zones (Instance, FromWall, FromSystem) instead of the server's local zone"},
	{regexp.MustCompile(`\btime\.Local\b`), "use chrono.Zones.Instance(); the server's zone isn't the instance's"},
	{regexp.MustCompile(`'localtime'`), "stored times aren't in the server's local zone; bound queries with Zones.ToSystem/ToWall"},
}

// TestTimeBoundary fails when Go code outside chrono converts times with
// the server's local zone.
func TestTimeBoundary(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			switch rel {
			case "chrono", ".git", ".go", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			code := line
			if c := strings.Index(code, "//"); c >= 0 {
				code = code[:c]
			}
			for _, r := range boundaryRules {
				if r.re.MatchString(code) {
					t.Errorf("%s:%d: %s\n\t%s", rel, i+1, r.why, strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
