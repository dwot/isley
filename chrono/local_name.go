package chrono

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	localNameOnce sync.Once
	localName     string
)

// LocalZoneName returns an IANA name for the server's zone (time.Local),
// whose own String() is just "Local". It tries $TZ, then the
// /etc/localtime symlink, and falls back to "UTC" when the server has no
// offset, else "Local".
func LocalZoneName() string {
	localNameOnce.Do(func() { localName = detectLocalName() })
	return localName
}

func detectLocalName() string {
	if tz := strings.TrimPrefix(os.Getenv("TZ"), ":"); tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	if target, err := filepath.EvalSymlinks("/etc/localtime"); err == nil {
		if i := strings.Index(target, "zoneinfo/"); i >= 0 {
			name := target[i+len("zoneinfo/"):]
			if _, err := time.LoadLocation(name); err == nil {
				return name
			}
		}
	}
	if _, offset := time.Now().Zone(); offset == 0 {
		return "UTC"
	}
	return "Local"
}

// InstanceName returns the IANA name of the instance zone.
func (z *Zones) InstanceName() string {
	loc := z.Instance()
	if loc == time.Local {
		return LocalZoneName()
	}
	return loc.String()
}
