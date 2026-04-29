package testutil

import "os"

// testPID returns the process id; isolated into its own file so the rest
// of the package does not pull in os just for a single call.
func testPID() int {
	return os.Getpid()
}
